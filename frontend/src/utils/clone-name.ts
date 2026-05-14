export function buildCloneName(name: string) {
  const baseName = String(name || '').trim() || '未命名'
  return `${baseName}-克隆`
}
