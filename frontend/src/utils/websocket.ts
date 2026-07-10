import request from '@/utils/request';

export type WSTicketPurpose =
  | 'server'
  | 'monitor'
  | 'terminal'
  | 'server_list'
  | 'life_probe_list'
  | 'life_probe_detail';

export interface WSTicketRequest {
  purpose: WSTicketPurpose;
  server_id?: number;
  life_probe_id?: number;
  session_id?: string;
}

export interface WSTicketResponse {
  ticket: string;
  expires_at: string;
}

export function issueWSTicket(input: WSTicketRequest): Promise<WSTicketResponse> {
  return request.post<WSTicketResponse>('/ws-tickets', input);
}

export function buildWebSocketURL(path: string, ticket?: string): string {
  const url = new URL(path, window.location.origin);
  url.protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  if (ticket) {
    url.searchParams.set('ticket', ticket);
  }
  return url.toString();
}

export async function buildAuthenticatedWebSocketURL(
  path: string,
  input: WSTicketRequest,
): Promise<string> {
  const response = await issueWSTicket(input);
  return buildWebSocketURL(path, response.ticket);
}
