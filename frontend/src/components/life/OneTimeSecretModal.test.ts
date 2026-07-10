import { mount } from '@vue/test-utils';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const messageMocks = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
}));

vi.mock('ant-design-vue', () => ({
  message: messageMocks,
}));

import OneTimeSecretModal from './OneTimeSecretModal.vue';

describe('OneTimeSecretModal', () => {
  beforeEach(() => {
    messageMocks.success.mockReset();
    messageMocks.error.mockReset();
  });

  it('shows and copies the one-time secret', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    });
    const wrapper = mount(OneTimeSecretModal, {
      props: { visible: true, secret: 'one-time-secret' },
      global: {
        stubs: {
          'a-modal': { template: '<div><slot /></div>' },
        },
      },
    });

    expect(wrapper.text()).toContain('one-time-secret');
    await wrapper.get('[data-testid="copy-secret"]').trigger('click');

    expect(writeText).toHaveBeenCalledWith('one-time-secret');
    expect(messageMocks.success).toHaveBeenCalled();
  });

  it('emits close without retaining a local copy', async () => {
    const wrapper = mount(OneTimeSecretModal, {
      props: { visible: true, secret: 'one-time-secret' },
      global: {
        stubs: {
          'a-modal': { template: '<div><slot /></div>' },
        },
      },
    });

    await wrapper.get('[data-testid="close-secret"]').trigger('click');

    expect(wrapper.emitted('close')).toHaveLength(1);
  });
});
