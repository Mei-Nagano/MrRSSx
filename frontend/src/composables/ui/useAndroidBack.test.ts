import { afterEach, describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import { defineComponent, h, nextTick, ref } from 'vue';
import { installAndroidBackBridge, useAndroidBack, useDismissOnBack } from './useAndroidBack';

function mountWithHandler(handler: () => boolean, priority = 0) {
  const component = defineComponent({
    setup() {
      useAndroidBack(handler, priority);
      return () => h('div');
    },
  });
  return mount(component);
}

afterEach(() => {
  delete window.__mrrssHandleBack;
});

describe('android back dispatch', () => {
  it('consults the highest priority handler first', () => {
    const calls: string[] = [];
    const low = mountWithHandler(() => {
      calls.push('low');
      return true;
    }, 10);
    const high = mountWithHandler(() => {
      calls.push('high');
      return true;
    }, 100);
    installAndroidBackBridge();

    expect(window.__mrrssHandleBack?.()).toBe(true);
    expect(calls).toEqual(['high']);
    high.unmount();
    low.unmount();
  });

  it('falls through unhandled handlers and reports an unhandled press', () => {
    const calls: string[] = [];
    const a = mountWithHandler(() => {
      calls.push('a');
      return false;
    }, 5);
    const b = mountWithHandler(() => {
      calls.push('b');
      return false;
    }, 1);
    installAndroidBackBridge();

    expect(window.__mrrssHandleBack?.()).toBe(false);
    expect(calls).toEqual(['a', 'b']);
    a.unmount();
    b.unmount();
  });

  it('drops handlers when their component unmounts', () => {
    let pressed = 0;
    const wrapper = mountWithHandler(() => {
      pressed += 1;
      return true;
    });
    installAndroidBackBridge();
    expect(window.__mrrssHandleBack?.()).toBe(true);
    wrapper.unmount();
    expect(window.__mrrssHandleBack?.()).toBe(false);
    expect(pressed).toBe(1);
  });
});

describe('useDismissOnBack', () => {
  function mountPopover(initial: boolean) {
    const open = ref(initial);
    const closes: number[] = [];
    const component = defineComponent({
      setup() {
        useDismissOnBack(open, () => {
          closes.push(1);
          open.value = false;
        });
        return () => h('div');
      },
    });
    return { open, closes, wrapper: mount(component) };
  }

  it('closes the popover and consumes the press while open', async () => {
    const { open, closes, wrapper } = mountPopover(true);
    installAndroidBackBridge();

    expect(window.__mrrssHandleBack?.()).toBe(true);
    expect(closes).toHaveLength(1);
    expect(open.value).toBe(false);
    wrapper.unmount();
  });

  it('lets the press through once the popover is closed', async () => {
    const { open, wrapper } = mountPopover(false);
    installAndroidBackBridge();

    expect(window.__mrrssHandleBack?.()).toBe(false);

    // Opening later still registers, so the handler is not limited to mount time.
    open.value = true;
    await nextTick();
    expect(window.__mrrssHandleBack?.()).toBe(true);
    expect(open.value).toBe(false);
    wrapper.unmount();
  });
});
