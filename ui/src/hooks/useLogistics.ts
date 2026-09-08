import { useQuery } from '@tanstack/react-query'
import { fetchLocationDetails, fetchLocations } from '../api/logistics'

export const LOGISTICS_LOCATIONS_KEY = ['logistics', 'locations'] as const

export function useLocations() {
  return useQuery({
    queryKey: LOGISTICS_LOCATIONS_KEY,
    queryFn: fetchLocations,
  })
}

export function useLocationDetails(name: string) {
  return useQuery({
    queryKey: ['logistics', 'location', name] as const,
    queryFn: () => fetchLocationDetails(name),
    enabled: Boolean(name),
  })
}
