import { describe, expect, it, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import { nextTick, ref } from 'vue';
import { usePullToRefresh } from './usePullToRefresh';

async function fixture() {
  const isRefreshing = ref(false);
  const onRefresh = vi.fn(() => {
    isRefreshing.value = true;
  });
  const container = ref<HTMLElement | null>(null);
  const wrapper = mount({
    setup() {
      const pull = usePullToRefresh(container, onRefresh, { isRefreshing });
      return { container, pull };
    },
    template: '<div ref="container" data-testid="scroller"></div>',
  });
  // The listener is bound by a post-flush watcher, so let the render settle
  // before dispatching touches.
  await nextTick();
  const element = wrapper.get('[data-testid="scroller"]').element as HTMLElement;

  function touch(type: 'touchstart' | 'touchmove' | 'touchend', y: number) {
    const point = { identifier: 1, clientX: 100, clientY: y };
    const event = new Event(type, { bubbles: true, cancelable: true });
    Object.defineProperties(event, {
      touches: { value: type === 'touchend' ? [] : [point] },
      changedTouches: { value: [point] },
    });
    element.dispatchEvent(event);
  }

  function scrollTo(top: number) {
    Object.defineProperty(element, 'scrollTop', { value: top, configurable: true });
  }

  const vm = wrapper.vm as unknown as { pull: { distance: { value: number } } };
  const distance = () => vm.pull.distance.value;

  return { wrapper, element, onRefresh, isRefreshing, touch, scrollTo, distance };
}

describe('pull to refresh', () => {
  it('refreshes once the pull passes the threshold', async () => {
    const f = await fixture();
    f.scrollTo(0);
    f.touch('touchstart', 100);
    f.touch('touchmove', 160);
    f.touch('touchmove', 260);
    f.touch('touchend', 260);

    expect(f.onRefresh).toHaveBeenCalledTimes(1);

    f.isRefreshing.value = false;
    await nextTick();
    expect(f.distance()).toBe(0);
    f.wrapper.unmount();
  });

  it('does nothing for a short pull or when the list is already scrolled', async () => {
    const f = await fixture();

    f.scrollTo(0);
    f.touch('touchstart', 100);
    f.touch('touchmove', 130);
    f.touch('touchend', 130);
    expect(f.onRefresh).not.toHaveBeenCalled();

    f.scrollTo(120);
    f.touch('touchstart', 100);
    f.touch('touchmove', 300);
    f.touch('touchend', 300);
    expect(f.onRefresh).not.toHaveBeenCalled();

    f.wrapper.unmount();
  });

  it('drops the gesture when it turns into an upward scroll', async () => {
    const f = await fixture();
    f.scrollTo(0);
    f.touch('touchstart', 200);
    f.touch('touchmove', 260);
    f.touch('touchmove', 120);
    f.touch('touchend', 120);

    expect(f.onRefresh).not.toHaveBeenCalled();
    expect(f.distance()).toBe(0);
    f.wrapper.unmount();
  });
});
