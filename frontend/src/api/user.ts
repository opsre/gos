import { http } from './http'
import type {
  PermissionListResponse,
  UserDataResponse,
  UserListResponse,
  UserManagerResponse,
  UserOptionListResponse,
  UserOrganizationResponse,
  UserParamPermissionListResponse,
  UserPermissionListResponse,
  UserRole,
  UserStatus,
} from '../types/user'

export interface UserListParams {
  username?: string
  name?: string
  role?: UserRole
  status?: UserStatus
  page?: number
  page_size?: number
}

export interface UserPayload {
  username?: string
  display_name: string
  email?: string
  phone?: string
  role: UserRole
  status: UserStatus
  password?: string
}

export interface UserPermissionPayload {
  items: Array<{
    permission_code: string
    scope_type?: string
    scope_value?: string
  }>
}

export interface UserParamPermissionPayload {
  param_key: string
  application_id?: string
  can_view: boolean
  can_edit: boolean
}

export async function listUsers(params: UserListParams): Promise<UserListResponse> {
  const response = await http.get<UserListResponse>('/users', { params })
  return response.data
}

export async function getUserByID(id: string): Promise<UserDataResponse> {
  const response = await http.get<UserDataResponse>(`/users/${id}`)
  return response.data
}

export async function createUser(payload: UserPayload): Promise<UserDataResponse> {
  const response = await http.post<UserDataResponse>('/users', payload)
  return response.data
}

export async function updateUser(id: string, payload: UserPayload): Promise<UserDataResponse> {
  const response = await http.put<UserDataResponse>(`/users/${id}`, payload)
  return response.data
}

export async function deleteUser(id: string): Promise<void> {
  await http.delete(`/users/${id}`)
}

export async function listUserOptions(): Promise<UserOptionListResponse> {
  const response = await http.get<UserOptionListResponse>('/users/options')
  return response.data
}

export async function getUserManager(userID: string): Promise<UserManagerResponse> {
  const response = await http.get<UserManagerResponse>(`/users/${userID}/manager`)
  return response.data
}

export async function updateUserManager(userID: string, managerUserID: string): Promise<UserManagerResponse> {
  const response = await http.put<UserManagerResponse>(`/users/${userID}/manager`, { manager_user_id: managerUserID })
  return response.data
}

export async function listUserOrganization(): Promise<UserOrganizationResponse> {
  const response = await http.get<UserOrganizationResponse>('/users/organization')
  return response.data
}

export async function listPermissions(): Promise<PermissionListResponse> {
  const response = await http.get<PermissionListResponse>('/permissions')
  return response.data
}

export async function listUserPermissions(userID: string): Promise<UserPermissionListResponse> {
  const response = await http.get<UserPermissionListResponse>(`/users/${userID}/permissions`)
  return response.data
}

export async function grantUserPermissions(userID: string, payload: UserPermissionPayload): Promise<void> {
  await http.post(`/users/${userID}/permissions`, payload)
}

export async function revokeUserPermissions(userID: string, payload: UserPermissionPayload): Promise<void> {
  await http.delete(`/users/${userID}/permissions`, { data: payload })
}

export async function listUserParamPermissions(userID: string, applicationID?: string): Promise<UserParamPermissionListResponse> {
  const response = await http.get<UserParamPermissionListResponse>(`/users/${userID}/param-permissions`, {
    params: {
      application_id: applicationID || undefined,
    },
  })
  return response.data
}

export async function upsertUserParamPermission(userID: string, payload: UserParamPermissionPayload, permissionID?: string): Promise<void> {
  if (permissionID) {
    await http.put(`/users/${userID}/param-permissions/${permissionID}`, payload)
    return
  }
  await http.post(`/users/${userID}/param-permissions`, payload)
}

export async function deleteUserParamPermission(userID: string, permissionID: string): Promise<void> {
  await http.delete(`/users/${userID}/param-permissions/${permissionID}`)
}
