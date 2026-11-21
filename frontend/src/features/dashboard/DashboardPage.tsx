import ReactECharts from 'echarts-for-react'

export function DashboardPage() {
  const chartOption = {
    tooltip: { trigger: 'axis' },
    grid: { left: 32, right: 16, top: 24, bottom: 24 },
    xAxis: { type: 'category', data: ['-6 h', '-5 h', '-4 h', '-3 h', '-2 h', '-1 h', 'Maintenant'] },
    yAxis: { type: 'value', max: 100 },
    series: [{ type: 'line', name: 'CPU', smooth: true, data: [18, 24, 21, 33, 28, 31, 26] }],
  }

  return (
    <main className="dashboard-shell">
      <aside className="sidebar">
        <strong>Homelab</strong>
        <nav aria-label="Navigation principale">
          <a className="active" href="/">Vue d'ensemble</a>
          <a href="/agents">Machines & agents</a>
          <a href="/metrics">Métriques</a>
          <a href="/network">Réseau</a>
          <a href="/settings">Paramètres</a>
        </nav>
      </aside>
      <section className="content">
        <header>
          <div>
            <p className="eyebrow">Vue d'ensemble</p>
            <h1>Infrastructure</h1>
          </div>
          <span className="status-pill">Manager opérationnel</span>
        </header>
        <div className="metrics-grid">
          <article><span>Agents</span><strong>0</strong><small>Aucun agent connecté</small></article>
          <article><span>CPU</span><strong>—</strong><small>En attente de données</small></article>
          <article><span>RAM</span><strong>—</strong><small>En attente de données</small></article>
          <article><span>Alertes</span><strong>0</strong><small>Sans alerte active</small></article>
        </div>
        <article className="chart-card">
          <h2>Charge CPU</h2>
          <ReactECharts option={chartOption} style={{ height: 300 }} />
        </article>
      </section>
    </main>
  )
}
