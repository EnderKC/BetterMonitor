import { flushPromises, mount } from '@vue/test-utils';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const apiMocks = vi.hoisted(() => ({
  createLifeProbe: vi.fn(),
  rotateLifeProbeSecret: vi.fn(),
}));

const requestMocks = vi.hoisted(() => ({
  put: vi.fn(),
  delete: vi.fn(),
}));

const modalMocks = vi.hoisted(() => ({
  confirm: vi.fn(),
}));

vi.mock('@/utils/lifeProbe', () => apiMocks);
vi.mock('@/utils/request', () => ({ default: requestMocks }));
vi.mock('@/utils/auth', () => ({ getToken: () => 'admin-token' }));
vi.mock('@/stores/uiStore', () => ({ useUIStore: () => ({ stopLoading: vi.fn() }) }));
vi.mock('vue-router', () => ({ useRouter: () => ({ push: vi.fn() }) }));
vi.mock('ant-design-vue', () => ({
  message: { success: vi.fn(), error: vi.fn() },
  Modal: modalMocks,
}));

import LifeProbeList from './LifeProbeList.vue';

class FakeWebSocket {
  static OPEN = 1;
  static CONNECTING = 0;
  static instances: FakeWebSocket[] = [];

  readyState = FakeWebSocket.OPEN;
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onerror: ((error: unknown) => void) | null = null;
  onclose: (() => void) | null = null;

  constructor(public url: string) {
    FakeWebSocket.instances.push(this);
  }

  send() {}
  close() {}
}

const globalMountOptions = {
  stubs: {
    'a-button': {
      emits: ['click'],
      template: '<button type="button" @click="$emit(\'click\')"><slot name="icon" /><slot /></button>',
    },
    'a-modal': {
      props: ['open', 'visible', 'title'],
      template:
        '<section v-if="open || visible" :data-title="title"><slot /><button data-testid="modal-ok" @click="$emit(\'ok\')">ok</button><button data-testid="modal-cancel" @click="$emit(\'cancel\')">cancel</button></section>',
    },
    'a-input': {
      props: ['value', 'placeholder'],
      emits: ['update:value'],
      template:
        '<input :placeholder="placeholder" :value="value" @input="$emit(\'update:value\', $event.target.value)" />',
    },
    'a-textarea': {
      props: ['value'],
      emits: ['update:value'],
      template:
        '<textarea :value="value" @input="$emit(\'update:value\', $event.target.value)" />',
    },
    'a-switch': true,
    'a-form': { template: '<form><slot /></form>' },
    'a-form-item': { template: '<div><slot /></div>' },
    'a-table': {
      props: ['dataSource'],
      template:
        '<div><div v-for="record in dataSource" :key="record.id"><slot name="bodyCell" :column="{ key: \'action\' }" :record="record" /></div></div>',
    },
    'a-space': { template: '<div><slot /></div>' },
    'a-tooltip': { props: ['title'], template: '<div :data-tooltip="title"><slot /></div>' },
    PlusOutlined: true,
    EditOutlined: true,
    DeleteOutlined: true,
    EyeOutlined: true,
    ReloadOutlined: true,
    KeyOutlined: true,
  },
};

const mountView = () => mount(LifeProbeList, { global: globalMountOptions });

const openCreateForm = async (wrapper: ReturnType<typeof mountView>) => {
  const addButton = wrapper.findAll('button').find((button) => button.text().includes('添加生命探针'));
  expect(addButton).toBeDefined();
  await addButton!.trigger('click');
};

describe('LifeProbeList one-time secret lifecycle', () => {
  beforeEach(() => {
    FakeWebSocket.instances = [];
    vi.stubGlobal('WebSocket', FakeWebSocket);
    apiMocks.createLifeProbe.mockReset();
    apiMocks.rotateLifeProbeSecret.mockReset();
    requestMocks.put.mockReset();
    requestMocks.delete.mockReset();
    modalMocks.confirm.mockReset();
  });

  it('shows the create secret once and clears it when the modal closes', async () => {
    apiMocks.createLifeProbe.mockResolvedValue({
      life_probe: { id: 1, ingest_secret_version: 1 },
      ingest_secret: 'created-secret',
    });
    const wrapper = mountView();

    await openCreateForm(wrapper);
    await wrapper.get('input[placeholder="例如：王小明 · iPhone"]').setValue('手表');
    await wrapper.get('input[placeholder="LifeLogger 客户端中的 Device ID"]').setValue('watch-1');
    await wrapper.get('section[data-title="添加生命探针"] [data-testid="modal-ok"]').trigger('click');
    await flushPromises();

    expect(apiMocks.createLifeProbe).toHaveBeenCalledWith(
      expect.objectContaining({ name: '手表', device_id: 'watch-1', allow_public_view: false }),
    );
    expect(wrapper.text()).toContain('created-secret');
    expect(JSON.stringify(localStorage)).not.toContain('created-secret');

    await wrapper.get('[data-testid="close-secret"]').trigger('click');
    expect(wrapper.text()).not.toContain('created-secret');
  });

  it('rotates an existing probe secret only after explicit confirmation', async () => {
    apiMocks.rotateLifeProbeSecret.mockResolvedValue({
      ingest_secret: 'rotated-secret',
      ingest_secret_version: 2,
    });
    const wrapper = mountView();
    const socket = FakeWebSocket.instances[0];
    socket.onmessage?.({
      data: JSON.stringify({
        type: 'life_probe_list',
        life_probes: [
          {
            id: 7,
            name: '手表',
            device_id: 'watch-1',
            allow_public_view: false,
          },
        ],
      }),
    });
    await flushPromises();

    const rotateAction = wrapper.find('[data-tooltip="轮换采集密钥"] button');
    expect(rotateAction.exists()).toBe(true);
    await rotateAction.trigger('click');
    expect(modalMocks.confirm).toHaveBeenCalledTimes(1);

    const confirmation = modalMocks.confirm.mock.calls[0][0];
    await confirmation.onOk();
    await flushPromises();

    expect(apiMocks.rotateLifeProbeSecret).toHaveBeenCalledWith(7);
    expect(wrapper.text()).toContain('rotated-secret');
  });

  it('renders one description field in the create form', async () => {
    const wrapper = mountView();

    await openCreateForm(wrapper);

    expect(wrapper.findAll('textarea')).toHaveLength(1);
  });
});
