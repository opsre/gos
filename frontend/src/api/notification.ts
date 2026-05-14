import { http } from './http'
import type {
  NotificationHookDataResponse,
  NotificationHookListParams,
  NotificationHookListResponse,
  NotificationHookPayload,
  NotificationMarkdownTemplateDataResponse,
  NotificationMarkdownTemplateListParams,
  NotificationMarkdownTemplateListResponse,
  NotificationMarkdownTemplatePayload,
  NotificationSourceDataResponse,
  NotificationSourceListParams,
  NotificationSourceListResponse,
  NotificationSourcePayload,
  NotificationSourceWebhookTestPayload,
  NotificationSourceWebhookTestResponse,
} from '../types/notification'

export async function listNotificationSources(params: NotificationSourceListParams = {}): Promise<NotificationSourceListResponse> {
  const response = await http.get<NotificationSourceListResponse>('/notification-sources', { params })
  return response.data
}

export async function getNotificationSource(id: string): Promise<NotificationSourceDataResponse> {
  const response = await http.get<NotificationSourceDataResponse>(`/notification-sources/${id}`)
  return response.data
}

export async function createNotificationSource(payload: NotificationSourcePayload): Promise<NotificationSourceDataResponse> {
  const response = await http.post<NotificationSourceDataResponse>('/notification-sources', payload)
  return response.data
}

export async function updateNotificationSource(id: string, payload: NotificationSourcePayload): Promise<NotificationSourceDataResponse> {
  const response = await http.put<NotificationSourceDataResponse>(`/notification-sources/${id}`, payload)
  return response.data
}

export async function deleteNotificationSource(id: string): Promise<{ ok: boolean }> {
  const response = await http.delete<{ ok: boolean }>(`/notification-sources/${id}`)
  return response.data
}

export async function testNotificationSourceWebhook(payload: NotificationSourceWebhookTestPayload): Promise<NotificationSourceWebhookTestResponse> {
  const response = await http.post<NotificationSourceWebhookTestResponse>('/notification-sources/actions/test-webhook', payload)
  return response.data
}

export async function listNotificationMarkdownTemplates(params: NotificationMarkdownTemplateListParams = {}): Promise<NotificationMarkdownTemplateListResponse> {
  const response = await http.get<NotificationMarkdownTemplateListResponse>('/notification-markdown-templates', { params })
  return response.data
}

export async function getNotificationMarkdownTemplate(id: string): Promise<NotificationMarkdownTemplateDataResponse> {
  const response = await http.get<NotificationMarkdownTemplateDataResponse>(`/notification-markdown-templates/${id}`)
  return response.data
}

export async function createNotificationMarkdownTemplate(payload: NotificationMarkdownTemplatePayload): Promise<NotificationMarkdownTemplateDataResponse> {
  const response = await http.post<NotificationMarkdownTemplateDataResponse>('/notification-markdown-templates', payload)
  return response.data
}

export async function updateNotificationMarkdownTemplate(id: string, payload: NotificationMarkdownTemplatePayload): Promise<NotificationMarkdownTemplateDataResponse> {
  const response = await http.put<NotificationMarkdownTemplateDataResponse>(`/notification-markdown-templates/${id}`, payload)
  return response.data
}

export async function deleteNotificationMarkdownTemplate(id: string): Promise<{ ok: boolean }> {
  const response = await http.delete<{ ok: boolean }>(`/notification-markdown-templates/${id}`)
  return response.data
}

export async function listNotificationHooks(params: NotificationHookListParams = {}): Promise<NotificationHookListResponse> {
  const response = await http.get<NotificationHookListResponse>('/notification-hooks', { params })
  return response.data
}

export async function getNotificationHook(id: string): Promise<NotificationHookDataResponse> {
  const response = await http.get<NotificationHookDataResponse>(`/notification-hooks/${id}`)
  return response.data
}

export async function createNotificationHook(payload: NotificationHookPayload): Promise<NotificationHookDataResponse> {
  const response = await http.post<NotificationHookDataResponse>('/notification-hooks', payload)
  return response.data
}

export async function updateNotificationHook(id: string, payload: NotificationHookPayload): Promise<NotificationHookDataResponse> {
  const response = await http.put<NotificationHookDataResponse>(`/notification-hooks/${id}`, payload)
  return response.data
}

export async function deleteNotificationHook(id: string): Promise<{ ok: boolean }> {
  const response = await http.delete<{ ok: boolean }>(`/notification-hooks/${id}`)
  return response.data
}
