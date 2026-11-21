# Stratégie de tests — Homelab Manager

Version 1.1 — 20 novembre 2026

## Backend Go

### Unitaires
- Tester chaque usecase et règle métier.
- Dépendances externes simulées via interfaces/ports.
- Tester les validations DTO, transitions d'état, droits et liste blanche.
- Tester les timeouts, annulations, erreurs et comportements de reconnexion.
- Couvrir les cas nominaux, limites et erreurs.

Commandes :
```bash
gofmt -w .
go vet ./...
go test ./...
go test -race ./...
```

### HTTP / Chi
- Utiliser `net/http/httptest`.
- Tester routes, méthodes, codes HTTP, middleware, authentification, autorisation, validation et format d'erreur.
- Tester les routes inconnues et payloads trop volumineux.
- Les tests ne doivent pas nécessiter un serveur HTTP externe.

### PostgreSQL / Ent
- Tests d'intégration contre une base PostgreSQL de test dédiée.
- Vérifier migrations, contraintes, index essentiels, transactions et repositories.
- Ne pas remplacer les tests d'intégration par SQLite : les comportements PostgreSQL doivent être testés sur PostgreSQL.

### WebSocket / Agent
- Tester handshake, authentification, version protocole, messages invalides, taille maximale, heartbeat, déconnexion, reconnexion, doublons et révocation.
- Tester que les actions non listées sont rejetées côté Agent.

## Frontend React

- Vitest + React Testing Library.
- Tester composants, hooks, formulaires React Hook Form, schémas Zod, états loading/empty/error/offline.
- Tester authentification, navigation, formulaires de configuration, opérations et affichage des métriques.
- Mock du client API et du flux WebSocket ; tests d'intégration ciblés du client temps réel.
- Vérifier accessibilité clavier et labels des formulaires.

Commandes attendues :
```bash
npm run lint
npm run typecheck
npm run test
npm run build
```

## Tests transverses
- Parcours login → tableau de bord → agent → opération → résultat.
- Agent hors ligne puis reconnexion.
- Agent révoqué.
- Données obsolètes et séries temporelles volumineuses.
- Contrôle d'accès et audit pour chaque action.
- Installation et démarrage sur Debian 13 via les unités systemd.
