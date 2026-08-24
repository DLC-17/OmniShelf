import { request } from './client'

export interface TimeSpentStats {
  minutesTv: number
  minutesMovie: number
  minutesBook: number
  minutesGame: number
}

export interface HeatmapEntry {
  date: string
  count: number
}

export interface Badge {
  id: string
  name: string
  description: string
  earnedAt: string
  icon: string
}

export async function getTimeSpent(): Promise<TimeSpentStats> {
  return request<TimeSpentStats>('/api/stats/time-spent')
}

export async function getHeatmap(): Promise<HeatmapEntry[]> {
  return request<HeatmapEntry[]>('/api/stats/heatmap')
}

export async function getBadges(): Promise<Badge[]> {
  return request<Badge[]>('/api/stats/badges')
}
