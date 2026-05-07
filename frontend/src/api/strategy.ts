import { http } from './http'
import type {
  StrategyTemplate,
  CreateStrategyTemplatePayload,
  UpdateStrategyTemplatePayload,
  K8sClusterRef,
  CreateK8sClusterRefPayload,
  UpdateK8sClusterRefPayload,
  AppEnvRuntimeBinding,
  CreateRuntimeBindingPayload,
  UpdateRuntimeBindingPayload,
  AppEnvStrategyBinding,
  CreateStrategyBindingPayload,
  UpdateStrategyBindingPayload,
  PaginatedData,
} from '../types/strategy'

export async function listStrategyTemplates(params?: {
  strategy_engine?: string
  strategy_type?: string
  status?: string
}): Promise<{ data: PaginatedData<StrategyTemplate> }> {
  const response = await http.get<{ data: PaginatedData<StrategyTemplate> }>(
    '/release-strategy-templates',
    { params },
  )
  return response.data
}

export async function getStrategyTemplate(id: string): Promise<{ data: StrategyTemplate }> {
  const response = await http.get<{ data: StrategyTemplate }>(
    `/release-strategy-templates/${id}`,
  )
  return response.data
}

export async function createStrategyTemplate(
  payload: CreateStrategyTemplatePayload,
): Promise<{ data: StrategyTemplate }> {
  const response = await http.post<{ data: StrategyTemplate }>(
    '/release-strategy-templates',
    payload,
  )
  return response.data
}

export async function updateStrategyTemplate(
  id: string,
  payload: UpdateStrategyTemplatePayload,
): Promise<{ data: StrategyTemplate }> {
  const response = await http.put<{ data: StrategyTemplate }>(
    `/release-strategy-templates/${id}`,
    payload,
  )
  return response.data
}

export async function deleteStrategyTemplate(id: string): Promise<void> {
  await http.delete(`/release-strategy-templates/${id}`)
}

export async function listK8sClusterRefs(): Promise<{
  data: PaginatedData<K8sClusterRef>
}> {
  const response = await http.get<{ data: PaginatedData<K8sClusterRef> }>(
    '/k8s-cluster-refs',
  )
  return response.data
}

export async function getK8sClusterRef(id: string): Promise<{
  data: K8sClusterRef
}> {
  const response = await http.get<{ data: K8sClusterRef }>(
    `/k8s-cluster-refs/${id}`,
  )
  return response.data
}

export async function createK8sClusterRef(
  payload: CreateK8sClusterRefPayload,
): Promise<{ data: K8sClusterRef }> {
  const response = await http.post<{ data: K8sClusterRef }>(
    '/k8s-cluster-refs',
    payload,
  )
  return response.data
}

export async function updateK8sClusterRef(
  id: string,
  payload: UpdateK8sClusterRefPayload,
): Promise<{ data: K8sClusterRef }> {
  const response = await http.put<{ data: K8sClusterRef }>(
    `/k8s-cluster-refs/${id}`,
    payload,
  )
  return response.data
}

export async function deleteK8sClusterRef(id: string): Promise<void> {
  await http.delete(`/k8s-cluster-refs/${id}`)
}

export async function listRuntimeBindings(params?: {
  application_id?: string
  env_code?: string
}): Promise<{ data: PaginatedData<AppEnvRuntimeBinding> }> {
  const response = await http.get<{ data: PaginatedData<AppEnvRuntimeBinding> }>(
    '/application-env-runtime-bindings',
    { params },
  )
  return response.data
}

export async function getRuntimeBinding(id: string): Promise<{
  data: AppEnvRuntimeBinding
}> {
  const response = await http.get<{ data: AppEnvRuntimeBinding }>(
    `/application-env-runtime-bindings/${id}`,
  )
  return response.data
}

export async function createRuntimeBinding(
  payload: CreateRuntimeBindingPayload,
): Promise<{ data: AppEnvRuntimeBinding }> {
  const response = await http.post<{ data: AppEnvRuntimeBinding }>(
    '/application-env-runtime-bindings',
    payload,
  )
  return response.data
}

export async function updateRuntimeBinding(
  id: string,
  payload: UpdateRuntimeBindingPayload,
): Promise<{ data: AppEnvRuntimeBinding }> {
  const response = await http.put<{ data: AppEnvRuntimeBinding }>(
    `/application-env-runtime-bindings/${id}`,
    payload,
  )
  return response.data
}

export async function deleteRuntimeBinding(id: string): Promise<void> {
  await http.delete(`/application-env-runtime-bindings/${id}`)
}

export async function listStrategyBindings(params?: {
  application_id?: string
  env_code?: string
}): Promise<{ data: PaginatedData<AppEnvStrategyBinding> }> {
  const response = await http.get<{ data: PaginatedData<AppEnvStrategyBinding> }>(
    '/application-env-strategy-bindings',
    { params },
  )
  return response.data
}

export async function getStrategyBinding(id: string): Promise<{
  data: AppEnvStrategyBinding
}> {
  const response = await http.get<{ data: AppEnvStrategyBinding }>(
    `/application-env-strategy-bindings/${id}`,
  )
  return response.data
}

export async function createStrategyBinding(
  payload: CreateStrategyBindingPayload,
): Promise<{ data: AppEnvStrategyBinding }> {
  const response = await http.post<{ data: AppEnvStrategyBinding }>(
    '/application-env-strategy-bindings',
    payload,
  )
  return response.data
}

export async function updateStrategyBinding(
  id: string,
  payload: UpdateStrategyBindingPayload,
): Promise<{ data: AppEnvStrategyBinding }> {
  const response = await http.put<{ data: AppEnvStrategyBinding }>(
    `/application-env-strategy-bindings/${id}`,
    payload,
  )
  return response.data
}

export async function deleteStrategyBinding(id: string): Promise<void> {
  await http.delete(`/application-env-strategy-bindings/${id}`)
}
