import { request } from './client'

export interface AdminUser {
  id: number
  username: string
  isAdmin: boolean
  mustChangePassword: boolean
  createdAt: string
}

export function fetchAdminUsers(): Promise<AdminUser[]> {
  return request<AdminUser[]>('/api/admin/users')
}

export function resetUserPassword(userId: number): Promise<{ message: string }> {
  return request<{ message: string }>(`/api/admin/users/${userId}/reset-password`, {
    method: 'POST',
  })
}

export function toggleUserAdmin(userId: number): Promise<{ isAdmin: boolean }> {
  return request<{ isAdmin: boolean }>(`/api/admin/users/${userId}/toggle-admin`, {
    method: 'POST',
  })
}
