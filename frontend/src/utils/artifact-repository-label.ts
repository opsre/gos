import type { ArtifactRepository } from '../types/artifact-repository'

// Object storage is identified by its bucket, while the file transfer types have
// no bucket at all and are identified by their host. Building this label in one
// place keeps every repository picker from degrading into "name ()" as new
// repository types are added.
export function formatArtifactRepositoryLabel(repository: ArtifactRepository) {
  const descriptor = repository.type === 'oss' ? repository.bucket : repository.endpoint
  return descriptor ? `${repository.name} (${descriptor})` : repository.name
}
