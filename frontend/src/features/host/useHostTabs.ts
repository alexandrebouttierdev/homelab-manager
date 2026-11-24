import { useSearchParams } from 'react-router-dom'
import type { SegmentedOption } from '../../components/ui/SegmentedTabs'

export type HostTab = 'overview' | 'vulnerabilities'

const options: ReadonlyArray<SegmentedOption<HostTab>> = [
  { value: 'overview', label: "Vue d'ensemble" },
  { value: 'vulnerabilities', label: 'Vulnérabilités' },
]

/** Section of the Machine page, kept in the address (?tab=vulnerabilities). */
export function useHostTabs() {
  const [params, setParams] = useSearchParams()
  const tab: HostTab = params.get('tab') === 'vulnerabilities' ? 'vulnerabilities' : 'overview'
  return {
    tab,
    options,
    setTab: (next: HostTab) =>
      setParams(next === 'overview' ? {} : { tab: next }, { replace: true }),
  }
}
