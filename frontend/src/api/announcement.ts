import { http } from "./http";
import type { AxiosRequestConfig } from "axios";
import type { Announcement, AnnouncementListResponse } from "../types/announcement";

export async function listAnnouncements(
  params?: Record<string, unknown>,
  config?: AxiosRequestConfig,
): Promise<AnnouncementListResponse> {
  const response = await http.get<AnnouncementListResponse>("/announcements", {
    params,
    ...config,
  });
  return response.data;
}

export async function listActiveAnnouncements(
  config?: AxiosRequestConfig,
): Promise<Announcement[]> {
  const response = await http.get<{ data: Announcement[] }>("/announcements/active", config);
  return response.data.data;
}

export async function getAnnouncementByID(
  id: string,
  config?: AxiosRequestConfig,
): Promise<Announcement> {
  const response = await http.get<{ data: Announcement }>(`/announcements/${id}`, config);
  return response.data.data;
}

export interface CreateAnnouncementPayload {
  title: string;
  content: string;
  enabled?: boolean;
  start_time: string;
  end_time: string;
}

export async function createAnnouncement(
  payload: CreateAnnouncementPayload,
): Promise<Announcement> {
  const response = await http.post<{ data: Announcement }>("/announcements", payload);
  return response.data.data;
}

export interface UpdateAnnouncementPayload {
  title: string;
  content: string;
  enabled?: boolean;
  start_time: string;
  end_time: string;
}

export async function updateAnnouncement(
  id: string,
  payload: UpdateAnnouncementPayload,
): Promise<Announcement> {
  const response = await http.put<{ data: Announcement }>(`/announcements/${id}`, payload);
  return response.data.data;
}

export async function toggleAnnouncement(
  id: string,
  enabled: boolean,
): Promise<Announcement> {
  const response = await http.put<{ data: Announcement }>(`/announcements/${id}/toggle`, { enabled });
  return response.data.data;
}

export async function deleteAnnouncement(id: string): Promise<void> {
  await http.delete(`/announcements/${id}`);
}
