import { http } from './http'
import type { ApplicationSetupStatus, OnboardingDraft, OnboardingInspection, OnboardingSession, OnboardingStatus } from '../types/onboarding'
import type { CreateReleaseOrderPayload } from '../types/release'

const base = '/onboarding/sessions'
export async function getOnboardingStatus(): Promise<OnboardingStatus> { return (await http.get('/onboarding/status')).data.data }
export async function createOnboardingSession(payload: {mode: string; application_id?: string; project_id?: string}): Promise<OnboardingSession> { return (await http.post(base, payload)).data.data }
export async function getOnboardingSession(id: string): Promise<OnboardingSession> { return (await http.get(`${base}/${encodeURIComponent(id)}`)).data.data }
export async function saveOnboardingDraft(session: OnboardingSession, draft: OnboardingDraft): Promise<OnboardingSession> { return (await http.put(`${base}/${session.id}/draft`, { expected_version: session.version, draft }, {timeout: 120_000})).data.data }
export async function getApplicationSetupStatus(id: string): Promise<ApplicationSetupStatus> { return (await http.get(`/applications/${encodeURIComponent(id)}/setup-status`, {timeout: 120_000})).data.data }
export async function inspectOnboarding(id: string): Promise<OnboardingInspection> { return (await http.post(`${base}/${id}/inspect`, {}, {timeout: 120_000})).data.data }
export async function applyOnboardingStep(session: OnboardingSession, step: string, key: string): Promise<OnboardingSession> { return (await http.post(`${base}/${session.id}/steps/${step}/apply`, { expected_version: session.version, request_key: key }, {timeout: 120_000})).data.data }
export async function checkOnboarding(id: string): Promise<OnboardingSession> { return (await http.post(`${base}/${id}/check`, {}, {timeout: 120_000})).data.data }
export async function abandonOnboarding(session: OnboardingSession): Promise<OnboardingSession> { return (await http.post(`${base}/${session.id}/abandon`, {expected_version: session.version})).data.data }
export async function createOnboardingFirstRelease(session: OnboardingSession, order: CreateReleaseOrderPayload, key: string): Promise<OnboardingSession> { return (await http.post(`${base}/${session.id}/first-release`, {expected_version: session.version, request_key: key, order}, {timeout: 120_000})).data.data }
