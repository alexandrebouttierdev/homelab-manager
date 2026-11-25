import type { UseFormRegisterReturn } from 'react-hook-form'
import { FormField } from '../alerts/FormField'

const hostKeyClass =
  'min-h-[64px] w-full rounded-md border border-line bg-surface px-3 py-2 font-mono text-xs text-text-primary outline-none focus:border-accent'

/** The pinned host key of an SFTP server, pasted by the administrator. */
export function HostKeyField({
  id,
  field,
  error,
  fingerprint,
}: {
  id: string
  field: UseFormRegisterReturn
  error?: string
  fingerprint: string | null
}) {
  return (
    <FormField
      id={id}
      label="Clé d'hôte du serveur"
      error={error}
      className="md:col-span-2"
      hint={
        fingerprint
          ? `Clé enregistrée : ${fingerprint}`
          : 'Contenu de /etc/ssh/ssh_host_ed25519_key.pub sur le serveur, ou une ligne de ssh-keyscan. Vérifiez-la sur le serveur lui-même : le Manager refusera tout serveur qui en présente une autre.'
      }
    >
      <textarea id={id} spellCheck={false} className={hostKeyClass} {...field} />
    </FormField>
  )
}
