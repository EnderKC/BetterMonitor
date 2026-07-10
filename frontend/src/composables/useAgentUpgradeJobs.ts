import { onMounted, onUnmounted, shallowReactive } from 'vue';
import {
  createAgentUpgrades,
  getAgentUpgradeJob,
  listAgentUpgradeJobs,
  type AgentUpgradeJob,
  type CreateAgentUpgradeRequest,
  type CreateAgentUpgradeResponse,
} from '../utils/version';

const pollIntervalMs = 2000;
const terminalStatuses = new Set<AgentUpgradeJob['status']>(['succeeded', 'failed', 'timed_out']);

interface AgentUpgradeJobOptions {
  onTerminal?: (job: AgentUpgradeJob) => void | Promise<void>;
}

export function useAgentUpgradeJobs(options: AgentUpgradeJobOptions = {}) {
  const jobs = shallowReactive(new Map<string, AgentUpgradeJob>());
  const timers = new Map<string, ReturnType<typeof setTimeout>>();
  const inFlight = new Set<string>();
  const terminalNotified = new Set<string>();
  let disposed = false;

  const isTerminal = (job: AgentUpgradeJob) => terminalStatuses.has(job.status);

  const clearTimer = (jobId: string) => {
    const timer = timers.get(jobId);
    if (timer !== undefined) {
      clearTimeout(timer);
      timers.delete(jobId);
    }
  };

  const notifyTerminal = (job: AgentUpgradeJob) => {
    if (terminalNotified.has(job.id)) return;
    terminalNotified.add(job.id);
    void options.onTerminal?.(job);
  };

  const schedule = (jobId: string) => {
    if (disposed || document.visibilityState === 'hidden' || timers.has(jobId)) return;
    const job = jobs.get(jobId);
    if (!job || isTerminal(job)) return;
    timers.set(jobId, setTimeout(() => {
      timers.delete(jobId);
      void refresh(jobId);
    }, pollIntervalMs));
  };

  const store = (job: AgentUpgradeJob) => {
    jobs.set(job.id, job);
    if (isTerminal(job)) {
      clearTimer(job.id);
      notifyTerminal(job);
      return;
    }
    schedule(job.id);
  };

  const refresh = async (jobId: string) => {
    if (disposed || inFlight.has(jobId)) return;
    inFlight.add(jobId);
    try {
      const job = await getAgentUpgradeJob(jobId);
      if (!disposed) store(job);
    } catch {
      if (!disposed) schedule(jobId);
    } finally {
      inFlight.delete(jobId);
    }
  };

  const track = async (jobOrId: AgentUpgradeJob | string) => {
    if (typeof jobOrId === 'string') {
      await refresh(jobOrId);
      return;
    }
    store(jobOrId);
  };

  const submit = async (request: CreateAgentUpgradeRequest): Promise<CreateAgentUpgradeResponse> => {
    const response = await createAgentUpgrades(request);
    for (const job of response.jobs) store(job);
    return response;
  };

  const loadActive = async (serverIds?: number[]) => {
    const allowed = serverIds ? new Set(serverIds) : undefined;
    const activeJobs = await listAgentUpgradeJobs({ active: true, limit: 500 });
    for (const job of activeJobs) {
      if (!allowed || allowed.has(job.server_id)) store(job);
    }
    return activeJobs;
  };

  const statusForServer = (serverId: number): AgentUpgradeJob | undefined => {
    let selected: AgentUpgradeJob | undefined;
    for (const job of jobs.values()) {
      if (job.server_id !== serverId) continue;
      if (!selected || job.updated_at > selected.updated_at) selected = job;
    }
    return selected;
  };

  const stop = (jobId?: string) => {
    if (jobId !== undefined) {
      clearTimer(jobId);
      return;
    }
    for (const id of timers.keys()) clearTimer(id);
  };

  const handleVisibilityChange = () => {
    if (document.visibilityState === 'hidden') {
      stop();
      return;
    }
    for (const job of jobs.values()) {
      if (!isTerminal(job)) schedule(job.id);
    }
  };

  onMounted(() => document.addEventListener('visibilitychange', handleVisibilityChange));
  onUnmounted(() => {
    disposed = true;
    stop();
    document.removeEventListener('visibilitychange', handleVisibilityChange);
  });

  return {
    jobs,
    submit,
    loadActive,
    track,
    statusForServer,
    stop,
  };
}
