import {
  GetAppPreferences,
  GetProfile,
  SaveAppPreferences,
  SaveProfile,
} from '../../bindings/portico/internal/interfaces/wails/routingfacade'
import type {
  AppPreferences,
  Provider,
  RouteProfile,
} from '../../bindings/portico/internal/routing/domain/models'

export type ProviderRecord = Provider
export type { AppPreferences, RouteProfile }

export const fetchRouteProfile = async (platform: string): Promise<RouteProfile> => {
  return GetProfile(platform)
}

export const saveRouteProfile = async (profile: RouteProfile): Promise<RouteProfile> => {
  return SaveProfile(profile)
}

export const fetchAppPreferences = async (): Promise<AppPreferences> => {
  return GetAppPreferences()
}

export const saveAppPreferences = async (preferences: AppPreferences): Promise<AppPreferences> => {
  return SaveAppPreferences(preferences)
}
