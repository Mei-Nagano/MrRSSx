import { afterEach, describe, expect, it } from 'vitest';
import { mount } from '@vue/test-utils';
import Sidebar from './Sidebar.vue';
import feedListSource from './FeedList.vue?raw';

afterEach(() => localStorage.removeItem('FeedListPinned'));

describe('floating feed list', () => {
  it.each(['pointerdown', 'focusin', 'blur'])(
    'closes on outside %s while preserving pinned lists',
    async (event) => {
      for (const pinned of [true, false]) {
        localStorage.setItem('FeedListPinned', String(pinned));
        const wrapper = mount(Sidebar, {
          props: { isOpen: true },
          attachTo: document.body,
          global: { stubs: { ActivityBar: true, FeedList: true } },
        });
        wrapper.element.dispatchEvent(new Event('pointerdown', { bubbles: true }));
        expect(wrapper.emitted('toggle')).toBeUndefined();
        (event === 'blur' ? window : document.body).dispatchEvent(
          new Event(event, { bubbles: true })
        );
        expect(wrapper.emitted('toggle')?.length ?? 0).toBe(pinned ? 0 : 1);
        wrapper.unmount();
      }
    }
  );
  it('removes outside listeners when unmounted', () => {
    localStorage.setItem('FeedListPinned', 'false');
    const wrapper = mount(Sidebar, {
      props: { isOpen: true },
      global: { stubs: { ActivityBar: true, FeedList: true } },
    });
    wrapper.unmount();
    document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }));
    expect(wrapper.emitted('toggle')).toBeUndefined();
  });

  // The saved-filter dialog is Teleported to body, so a tap inside it used to be
  // read as "outside the drawer" and collapsed the sheet with the dialog on top.
  it('keeps the sheet for a dialog Teleported out of the drawer', () => {
    localStorage.setItem('FeedListPinned', 'false');
    const popover = document.createElement('div');
    popover.setAttribute('data-drawer-popover', '');
    const control = document.createElement('button');
    popover.appendChild(control);
    document.body.appendChild(popover);

    const wrapper = mount(Sidebar, {
      props: { isOpen: true },
      attachTo: document.body,
      global: { stubs: { ActivityBar: true, FeedList: true } },
    });

    control.dispatchEvent(new Event('pointerdown', { bubbles: true }));
    expect(wrapper.emitted('toggle')).toBeUndefined();

    // A press that belongs to neither the drawer nor its dialog still closes it.
    document.body.dispatchEvent(new Event('pointerdown', { bubbles: true }));
    expect(wrapper.emitted('toggle')?.length ?? 0).toBe(1);

    wrapper.unmount();
    popover.remove();
  });

  it('marks every drawer-owned dialog as belonging to the drawer', () => {
    // Both the save and edit dialogs are Teleported, so both need the marker; a
    // missing one silently collapses the drawer on the first tap inside it.
    const marks = feedListSource.match(/data-drawer-popover/g) ?? [];
    expect(marks.length).toBeGreaterThanOrEqual(2);
  });
});
