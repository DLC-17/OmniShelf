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

export interface CompletionBreakdown {
  episodes: number
  movies: number
  games: number
  books: number
  albums: number
}

export interface WrappedTimeSpent {
  totalMinutes: number
  totalHours: number
  totalDays: number
  formatted: string
  minutesTv: number
  minutesMovie: number
  minutesBook: number
  minutesGame: number
  minutesMusic: number
}

export interface CreatorStat {
  name: string
  category: string
  count: number
}

export interface GenreStat {
  name: string
  count: number
}

export interface MonthlyHeatmapEntry {
  month: number
  monthName: string
  count: number
  breakdown: CompletionBreakdown
}

export interface BusiestMonth {
  month: number
  monthName: string
  count: number
}

export interface PhysicalCollectionStats {
  totalValue: number
  totalItems: number
  cardValue: number
  cardCount: number
  vinylValue: number
  vinylCount: number
  gameValue: number
  gameCount: number
}

export interface WrappedResponse {
  year: number
  totalCompleted: number
  breakdown: CompletionBreakdown
  timeSpent: WrappedTimeSpent
  topCreators: CreatorStat[]
  topGenres: GenreStat[]
  monthlyHeatmap: MonthlyHeatmapEntry[]
  busiestMonth: BusiestMonth
  physicalCollection: PhysicalCollectionStats
}

export async function getWrapped(year?: number): Promise<WrappedResponse> {
  const query = year ? `?year=${year}` : ''
  return request<WrappedResponse>(`/api/stats/wrapped${query}`)
}

