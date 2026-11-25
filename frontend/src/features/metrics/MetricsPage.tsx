import { Activity, BarChart3, RefreshCw } from 'lucide-react'
import { Chart } from '../../components/charts/Chart'
import { Badge } from '../../components/ui/Badge'
import { Button } from '../../components/ui/Button'
import { Card, CardBody, CardFooter, CardHeader } from '../../components/ui/Card'
import { Table, TBody, TD, TH, THead, TR } from '../../components/ui/DataTable'
import { EmptyState } from '../../components/ui/EmptyState'
import { ErrorState } from '../../components/ui/ErrorState'
import { LoadingState } from '../../components/ui/LoadingState'
import { PageHeader } from '../../components/ui/PageHeader'
import { SegmentedTabs } from '../../components/ui/SegmentedTabs'
import { Select } from '../../components/ui/Select'
import { formatDateTime, formatMetricValue } from '../../lib/format'
import { labelsText } from '../../lib/metrics'
import { ranges, useMetricsPage } from './useMetricsPage'

export function MetricsPage() {
  const {
    metricName,
    setMetricName,
    setLabelValue,
    range,
    setRange,
    definition,
    labels,
    effectiveLabel,
    history,
    samples,
    latest,
    minimum,
    maximum,
    mean,
    chartOption,
    groups,
  } = useMetricsPage()

  return (
    <div className="space-y-4">
      <PageHeader
        title="Métriques"
        breadcrumb={[{ label: 'Système' }, { label: 'Métriques' }]}
        badge={<Badge tone="neutral">{samples.length} échantillon(s)</Badge>}
        description="Historiques bruts des métriques de l'hôte. Les échantillons sont bornés par la période sélectionnée et la rétention configurée."
        actions={
          <Button
            size="sm"
            variant="secondary"
            icon={<RefreshCw className="h-3.5 w-3.5" />}
            onClick={() => {
              void history.refetch()
            }}
          >
            Actualiser
          </Button>
        }
      />

      <div className="flex flex-wrap items-center gap-2 rounded-lg border border-line bg-card p-2.5">
        <Select
          aria-label="Choisir une métrique"
          value={metricName}
          onChange={(event) => {
            setMetricName(event.target.value)
            setLabelValue('')
          }}
          className="min-w-[260px]"
        >
          {groups.map(({ group, metrics: groupMetrics }) => (
            <optgroup key={group} label={group}>
              {groupMetrics.map((metric) => (
                <option key={metric.name} value={metric.name}>
                  {metric.label}
                </option>
              ))}
            </optgroup>
          ))}
        </Select>

        {definition.labelKey && labels.length > 1 && (
          <Select
            aria-label={`Choisir ${definition.labelKey}`}
            value={effectiveLabel}
            onChange={(event) => setLabelValue(event.target.value)}
            className="min-w-[170px]"
          >
            {labels.map((label) => (
              <option key={label} value={label}>
                {label}
              </option>
            ))}
          </Select>
        )}

        <span className="font-mono text-xs text-text-muted">{definition.name}</span>

        <div className="ml-auto">
          <SegmentedTabs
            ariaLabel="Période de l'historique"
            value={range}
            onChange={setRange}
            options={ranges.map((item) => ({ value: item.value, label: item.label }))}
          />
        </div>
      </div>

      {history.isError && (
        <ErrorState
          message={history.error.message}
          onRetry={() => {
            void history.refetch()
          }}
        />
      )}
      {history.isLoading && !history.isError && (
        <LoadingState label="Chargement de l'historique…" rows={2} />
      )}

      {!history.isLoading && !history.isError && samples.length === 0 && (
        <EmptyState
          icon={BarChart3}
          title="Aucun échantillon"
          description="Cette métrique n'a pas encore été collectée sur la période sélectionnée, ou n'est pas disponible sur cet hôte."
        />
      )}

      {samples.length > 0 && (
        <>
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            <Stat
              label="Dernière valeur"
              value={formatMetricValue(latest?.value, definition.unit)}
              hint={latest ? formatDateTime(latest.recordedAt) : '—'}
            />
            <Stat
              label="Moyenne"
              value={formatMetricValue(mean, definition.unit)}
              hint={`${samples.length} échantillon(s)`}
            />
            <Stat
              label="Minimum"
              value={formatMetricValue(minimum, definition.unit)}
              hint="Sur la période"
            />
            <Stat
              label="Maximum"
              value={formatMetricValue(maximum, definition.unit)}
              hint="Sur la période"
            />
          </div>

          <Card>
            <CardHeader
              title={definition.label}
              subtitle={`${range.replace('h', ' heure(s)').replace('7d', '7 jours')} · ${effectiveLabel === 'global' ? 'série globale' : `${definition.labelKey} ${effectiveLabel}`}`}
              icon={<Activity className="h-3.5 w-3.5" />}
              actions={<Badge tone="neutral">{definition.unit}</Badge>}
            />
            <CardBody>
              <Chart
                option={chartOption}
                height={320}
                label={`Historique de ${definition.label}`}
              />
            </CardBody>
            <CardFooter className="flex flex-wrap items-center justify-between gap-2 text-xs text-text-muted">
              <span>{samples.length} point(s) · résolution brute</span>
              <span>Les valeurs manquantes ne sont pas interpolées.</span>
            </CardFooter>
          </Card>

          <Card>
            <CardHeader
              title="Derniers échantillons"
              subtitle="Les 20 valeurs les plus récentes"
              icon={<BarChart3 className="h-3.5 w-3.5" />}
            />
            <CardBody className="p-0">
              <Table minWidthClassName="min-w-[520px]" className="rounded-none border-0">
                <THead>
                  <tr>
                    <TH>Horodatage</TH>
                    <TH>Valeur</TH>
                    <TH>Étiquettes</TH>
                  </tr>
                </THead>
                <TBody>
                  {[...samples]
                    .reverse()
                    .slice(0, 20)
                    .map((sample) => (
                      <TR key={`${sample.recordedAt}-${sample.value}`}>
                        <TD className="font-mono text-xs">{formatDateTime(sample.recordedAt)}</TD>
                        <TD className="font-mono text-xs text-text-primary">
                          {formatMetricValue(sample.value, sample.unit)}
                        </TD>
                        <TD className="font-mono text-xs text-text-muted">
                          {labelsText(sample.labels)}
                        </TD>
                      </TR>
                    ))}
                </TBody>
              </Table>
            </CardBody>
          </Card>
        </>
      )}
    </div>
  )
}

function Stat({ label, value, hint }: { label: string; value: string; hint: string }) {
  return (
    <div className="rounded-lg border border-line bg-card p-4">
      <p className="text-xs text-text-muted">{label}</p>
      <p className="mt-1 text-xl font-semibold tracking-[-0.02em] text-text-primary">{value}</p>
      <p className="mt-1 text-xs text-text-muted">{hint}</p>
    </div>
  )
}
