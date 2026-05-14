import test from 'node:test'
import assert from 'node:assert/strict'
import { existsSync, readFileSync } from 'node:fs'

const routerURL = new URL('../src/router/index.ts', import.meta.url)
const layoutURL = new URL('../src/layouts/AppLayout.vue', import.meta.url)
const artifactConfigURL = new URL('../src/views/artifact/ArtifactRepositoryConfigView.vue', import.meta.url)
const routerSource = readFileSync(routerURL, 'utf8')
const layoutSource = readFileSync(layoutURL, 'utf8')

test('artifact center exposes repository configuration navigation', () => {
  assert.ok(existsSync(artifactConfigURL), 'artifact repository configuration page should exist')
  assert.match(
    routerSource,
    /const ArtifactRepositoryConfigView = \(\) => import\('\.\.\/views\/artifact\/ArtifactRepositoryConfigView\.vue'\)/,
    'router should lazy-load the artifact repository configuration page',
  )
  assert.match(
    routerSource,
    /path:\s*'\/artifacts\/repositories'[\s\S]*name:\s*'artifact-repository-config'[\s\S]*component:\s*ArtifactRepositoryConfigView[\s\S]*meta:\s*\{\s*title:\s*'制品配置',\s*permission:\s*'artifact_repo\.manage'\s*\}/,
    'router should expose the artifact repository configuration route',
  )
  assert.match(
    layoutSource,
    /route\.path\.startsWith\('\/artifacts\/repositories'\)[\s\S]*return \['artifact-repository-config'\]/,
    'sidebar should activate the artifact configuration item',
  )
  assert.match(
    layoutSource,
    /route\.path\.startsWith\('\/artifacts'\)[\s\S]*return \['artifact-center'\]/,
    'sidebar should expand the artifact center group',
  )
  assert.match(
    layoutSource,
    /function goToArtifactRepositoryConfig\(\)[\s\S]*router\.push\('\/artifacts\/repositories'\)/,
    'sidebar should navigate to artifact repository configuration',
  )
  assert.match(
    layoutSource,
    /key="artifact-center"[\s\S]*<template #title>制品中心<\/template>[\s\S]*key="artifact-repository-config"[\s\S]*@click="goToArtifactRepositoryConfig"[\s\S]*制品配置/,
    'sidebar should render the artifact center first-level menu and configuration child item',
  )
})
