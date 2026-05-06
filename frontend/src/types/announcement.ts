export interface Announcement {
  id: string;
  title: string;
  content: string;
  enabled: boolean;
  start_time: string;
  end_time: string;
  created_by: string;
  updated_by: string;
  created_at: string;
  updated_at: string;
}

export interface AnnouncementListResponse {
  data: Announcement[];
  page: number;
  page_size: number;
  total: number;
}
