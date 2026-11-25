// Mirror of the server rules for a repository (API_CONTRACTS.md § 4.10):
// it guides the input, the server decides.
export const absolutePath = /^\/(?!.*\/\/)(?!.*\/$)(?!.*(^|\/)\.\.(\/|$)).*$/
const sftpLocation =
  /^sftp:\/\/[A-Za-z_][A-Za-z0-9._-]{0,63}@(\[[0-9A-Fa-f:.]+\]|[A-Za-z0-9][A-Za-z0-9.-]*)(:\d{1,5})?(\/[A-Za-z0-9._+@=,-]+)+$/

export type RepositoryIssue = { path: 'location' | 'hostKey'; message: string }

/** What the server would refuse in a repository; the server decides. */
export function repositoryIssues(
  type: string,
  location: string,
  hostKey: string,
): RepositoryIssue[] {
  const issues: RepositoryIssue[] = []
  if (type === 'local' && (!absolutePath.test(location) || location === '/'))
    issues.push({ path: 'location', message: 'Chemin absolu, sans « .. » ni « / » final' })
  if ((type === 'rest' || type === 's3') && !/^https:\/\/[^/@?#\s]+/.test(location))
    issues.push({ path: 'location', message: 'Adresse https://, sans identifiants' })
  if (type === 's3' && !/^https:\/\/[^/]+\/[^/]+/.test(location))
    issues.push({ path: 'location', message: 'Indiquez le bucket : https://hôte/bucket[/préfixe]' })
  if (type === 'sftp') {
    const path = location.replace(/^sftp:\/\/[^/]*/, '')
    if (!sftpLocation.test(location) || !absolutePath.test(path))
      issues.push({
        path: 'location',
        message: 'Adresse sftp://utilisateur@hôte[:port]/chemin/absolu, sans mot de passe',
      })
    if (hostKey.trim() === '')
      issues.push({ path: 'hostKey', message: "La clé d'hôte du serveur est requise" })
  }
  return issues
}
