import {
  DeleteModelMappingScheme,
  GetAppPreferences,
  GetProfile,
  SaveAppPreferences,
  SaveModelMappingScheme,
  SaveProfile,
  SetActiveModelMappingScheme,
} from '../../bindings/portico/internal/interfaces/wails/routingfacade'
import type {
  AppPreferences,
  ModelMappingScheme,
  Provider,
  RouteProfile,
} from '../../bindings/portico/internal/routing/domain/models'

export type ProviderRecord = Provider
export type { AppPreferences, ModelMappingScheme, RouteProfile }

export const fetchRouteProfile = async (platform: string): Promise<RouteProfile> => {
  return GetProfile(platform)
}

export const saveRouteProfile = async (profile: RouteProfile): Promise<RouteProfile> => {
  return SaveProfile(profile)
}

export const saveModelMappingScheme = async (
  platform: string,
  providerID: number,
  originalName: string,
  scheme: ModelMappingScheme,
  activate: boolean,
): Promise<RouteProfile> => {
  return SaveModelMappingScheme(platform, providerID, originalName, scheme, activate)
}

export const deleteModelMappingScheme = async (
  platform: string,
  providerID: number,
  name: string,
): Promise<RouteProfile> => {
  return DeleteModelMappingScheme(platform, providerID, name)
}

export const setActiveModelMappingScheme = async (
  platform: string,
  providerID: number,
  name: string,
): Promise<RouteProfile> => {
  return SetActiveModelMappingScheme(platform, providerID, name)
}

export const fetchAppPreferences = async (): Promise<AppPreferences> => {
  return GetAppPreferences()
}

export const saveAppPreferences = async (preferences: AppPreferences): Promise<AppPreferences> => {
  return SaveAppPreferences(preferences)
}
