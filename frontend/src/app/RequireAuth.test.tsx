import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { RequireAuth } from './RequireAuth'

const session = vi.hoisted(() => ({
  user: {
    id: 'a',
    username: 'alice',
    displayName: 'Alice',
    role: 'viewer' as const,
    mustChangePassword: true,
  },
  isPending: false,
  unauthenticated: false,
  error: null,
  mustChangePassword: true,
  roleLabel: 'Lecteur',
  canManageUsers: false,
  canManageSettings: false,
  canManagePower: false,
}))

vi.mock('../hooks/useSession', () => ({ useSession: () => session }))

describe('RequireAuth', () => {
  it('sends an account with an initial password to the password change', () => {
    render(
      <MemoryRouter initialEntries={['/']}>
        <Routes>
          <Route
            path="/"
            element={
              <RequireAuth>
                <p>Contenu protégé</p>
              </RequireAuth>
            }
          />
          <Route path="/password-change" element={<p>Changement imposé</p>} />
        </Routes>
      </MemoryRouter>,
    )
    expect(screen.getByText('Changement imposé')).toBeInTheDocument()
    expect(screen.queryByText('Contenu protégé')).not.toBeInTheDocument()
  })

  it('sends an administrator without a required second factor to its enrollment', () => {
    session.mustChangePassword = false
    Object.assign(session, { mustEnrollSecondFactor: true })
    render(
      <MemoryRouter initialEntries={['/']}>
        <Routes>
          <Route
            path="/"
            element={
              <RequireAuth>
                <p>Contenu protégé</p>
              </RequireAuth>
            }
          />
          <Route path="/second-factor-setup" element={<p>Enrôlement imposé</p>} />
        </Routes>
      </MemoryRouter>,
    )
    expect(screen.getByText('Enrôlement imposé')).toBeInTheDocument()
  })
})
