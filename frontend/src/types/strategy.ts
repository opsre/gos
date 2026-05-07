export interface StrategyTemplate {
  id: string
  name: string
  strategy_engine: string
  strategy_type: string
  strategy_config: string
  description: string
  status: string
  created_at: string
  updated_at: string
}

export interface CreateStrategyTemplatePayload {
  name: string
  strategy_engine: string
  strategy_type: string
  strategy_config: string
  description: string
}

export interface UpdateStrategyTemplatePayload {
  name?: string
  strategy_engine?: string
  strategy_type?: string
  strategy_config?: string
  description?: string
  status?: string
}

export interface K8sClusterRef {
  id: string
  code: string
  cluster_name: string
  environment_code: string
  api_server: string
  default_namespace: string
  access_mode: string
  argocd_instance_id: string
  supports_native_strategy: boolean
  supports_rollouts: boolean
  traffic_provider: string
  created_at: string
  updated_at: string
}

export interface CreateK8sClusterRefPayload {
  code: string
  cluster_name: string
  environment_code: string
  api_server: string
  default_namespace: string
  argocd_instance_id: string
  supports_native_strategy: boolean
  supports_rollouts: boolean
  traffic_provider: string
}

export interface UpdateK8sClusterRefPayload {
  code?: string
  cluster_name?: string
  environment_code?: string
  api_server?: string
  default_namespace?: string
  argocd_instance_id?: string
  supports_native_strategy?: boolean
  supports_rollouts?: boolean
  traffic_provider?: string
}

export interface AppEnvRuntimeBinding {
  id: string
  application_id: string
  env_code: string
  k8s_cluster_ref_id: string
  namespace: string
  workload_name: string
  created_at: string
  updated_at: string
}

export interface CreateRuntimeBindingPayload {
  application_id: string
  env_code: string
  k8s_cluster_ref_id: string
  namespace: string
  workload_name: string
}

export interface UpdateRuntimeBindingPayload {
  k8s_cluster_ref_id?: string
  namespace?: string
  workload_name?: string
}

export interface AppEnvStrategyBinding {
  id: string
  application_id: string
  env_code: string
  strategy_template_id: string
  overrides_config: string
  created_at: string
  updated_at: string
}

export interface CreateStrategyBindingPayload {
  application_id: string
  env_code: string
  strategy_template_id: string
  overrides_config: string
}

export interface UpdateStrategyBindingPayload {
  overrides_config?: string
}

export interface PaginatedData<T> {
  items: T[]
  total: number
}
