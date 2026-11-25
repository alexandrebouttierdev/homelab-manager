import { KeyRound } from 'lucide-react'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardHeader } from '../../components/ui/Card'
import { formatDateTime } from '../../lib/format'
import type { BackupSSHKeyModel } from './useBackupSSHKey'

/** The SSH key the Manager logs in to its SFTP repositories with. */
export function BackupSSHKeyCard({ model }: { model: BackupSSHKeyModel }) {
  if (!model.visible) return null
  return (
    <Card>
      <CardHeader
        title="Clé SSH du Manager"
        subtitle="Pour se connecter aux dépôts SFTP"
        icon={<KeyRound className="h-3.5 w-3.5" />}
      />
      <CardBody className="space-y-3">
        <p className="text-xs text-text-secondary">
          Le Manager se connecte aux serveurs SFTP avec sa propre clé, jamais avec celle de root.
          Sur le serveur, ajoutez la clé publique au fichier <code>~/.ssh/authorized_keys</code> du
          compte indiqué dans l&apos;adresse du dépôt ; un compte dédié, limité à SFTP, est
          préférable. La clé privée reste chiffrée dans le coffre du Manager.
        </p>
        {model.key ? (
          <div className="space-y-2">
            <label htmlFor="backup-ssh-public-key" className="block text-xs text-text-secondary">
              Clé publique
            </label>
            <textarea
              id="backup-ssh-public-key"
              readOnly
              spellCheck={false}
              value={model.key.publicKey}
              onFocus={(event) => event.currentTarget.select()}
              className="min-h-[56px] w-full rounded-md border border-line bg-surface px-3 py-2 font-mono text-xs text-text-primary outline-none"
            />
            <p className="text-xs text-text-muted">
              Empreinte {model.key.fingerprint} · créée le {formatDateTime(model.key.createdAt)}
            </p>
          </div>
        ) : (
          <p className="text-xs text-text-secondary">
            Aucune clé pour l&apos;instant : générez-la avant d&apos;enregistrer un dépôt SFTP.
          </p>
        )}
        {model.canManage && (
          <div className="flex flex-wrap gap-2">
            {model.canCopy && (
              <Button size="sm" onClick={model.copyPublicKey}>
                {model.copied ? 'Copiée' : 'Copier la clé publique'}
              </Button>
            )}
            <Button
              size="sm"
              variant={model.key ? 'danger' : 'primary'}
              loading={model.isGenerating && !model.confirmReplace}
              disabled={!model.secretsAvailable || model.confirmReplace}
              onClick={model.generate}
            >
              {model.key ? 'Remplacer la clé' : 'Générer la clé SSH'}
            </Button>
          </div>
        )}
        {model.confirmReplace && (
          <div
            className="space-y-2 rounded-md border border-danger/30 bg-danger/5 p-3"
            role="alert"
          >
            <p className="text-xs text-text-secondary">
              L&apos;ancienne clé ne servira plus : les sauvegardes SFTP échoueront tant que la
              nouvelle clé publique ne sera pas installée sur chaque serveur.
            </p>
            <div className="flex gap-2">
              <Button
                size="sm"
                variant="danger"
                loading={model.isGenerating}
                onClick={model.replace}
              >
                Confirmer le remplacement
              </Button>
              <Button size="sm" onClick={model.cancelReplace}>
                Annuler
              </Button>
            </div>
          </div>
        )}
        {model.generateError && (
          <p className="text-xs text-danger" role="alert">
            {model.generateError}
          </p>
        )}
      </CardBody>
    </Card>
  )
}
