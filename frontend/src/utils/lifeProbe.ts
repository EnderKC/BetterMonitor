import request from './request';

export interface LifeProbeInput {
  name: string;
  device_id: string;
  description: string;
  tags: string;
  allow_public_view: boolean;
}

export interface CreateLifeProbeResponse {
  life_probe: { id: number; ingest_secret_version?: number };
  ingest_secret: string;
}

export interface RotateLifeProbeSecretResponse {
  ingest_secret: string;
  ingest_secret_version: number;
}

export function createLifeProbe(payload: LifeProbeInput): Promise<CreateLifeProbeResponse> {
  return request.post('/life-probes', payload);
}

export function rotateLifeProbeSecret(id: number): Promise<RotateLifeProbeSecretResponse> {
  return request.post(`/life-probes/${id}/rotate-secret`);
}
