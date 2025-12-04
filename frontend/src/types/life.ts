export interface HeartRatePoint {
  time: string;
  value: number;
}

export interface StepSamplePoint {
  start: string;
  end: string;
  value: number;
  sample_type: string;
}

export interface FocusEventPoint {
  time: string;
  is_focused: boolean;
  change_reason: string;
}

export interface ScreenEventPoint {
  time: string;
  action: string;
}

export interface SleepSegmentPoint {
  stage: string;
  start_time: string;
  end_time: string;
  duration: number;
}

export interface SleepOverview {
  total_duration: number;
  stage_durations: Record<string, number>;
  start_time?: string | null;
  end_time?: string | null;
}

export interface DailyStepPoint {
  day: string;
  total: number;
  sample_type: string;
}

export interface LifeProbeSummary {
  id: number;
  name: string;
  device_id: string;
  description?: string;
  tags?: string;
  allow_public_view: boolean;
  battery_level?: number;
  last_sync_at?: string;
  latest_heart_rate?: HeartRatePoint | null;
  focus_event?: FocusEventPoint | null;
  steps_today: number;
  sleep_duration?: number;
  daily_totals?: DailyStepPoint[];
}

export interface LifeProbeDetails {
  summary: LifeProbeSummary;
  heart_rates: HeartRatePoint[];
  step_samples: StepSamplePoint[];
  focus_events: FocusEventPoint[];
  screen_events: ScreenEventPoint[];
  sleep_segments: SleepSegmentPoint[];
  sleep_overview: SleepOverview;
}
