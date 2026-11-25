import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { loadState } from '../../hooks/loadState'
import { createApiToken, listApiTokens, revokeApiToken } from '../../lib/api/tokens'
import { tokenExpiry, tokenScopeLabel, tokenScopeOptions } from '../../lib/tokens'
import { apiTokenSchema, type ApiTokenValues } from '../../schemas/apiToken'

const tokensQueryKey = ['auth', 'tokens'] as const

/** API tokens of the signed-in account (SPEC_ADMINISTRATION.md § 3.5). */
export function useApiTokens() {
  const queryClient = useQueryClient()
  // The secret of the token just created, shown this once.
  const [secret, setSecret] = useState<string | null>(null)
  const [formOpen, setFormOpen] = useState(false)
  const query = useQuery({ queryKey: tokensQueryKey, queryFn: listApiTokens })
  const form = useForm<ApiTokenValues>({
    resolver: zodResolver(apiTokenSchema),
    defaultValues: { name: '', scopes: ['metrics:read'], expiresInDays: 90 },
  })
  const create = useMutation({
    mutationFn: (input: ApiTokenValues) => createApiToken(input),
    onSuccess: (result) => {
      setSecret(result.token)
      setFormOpen(false)
      form.reset()
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: tokensQueryKey }),
  })
  const revoke = useMutation({
    mutationFn: (id: string) => revokeApiToken(id),
    onSettled: () => queryClient.invalidateQueries({ queryKey: tokensQueryKey }),
  })
  const rows = useMemo(() => {
    const now = new Date()
    return (query.data?.tokens ?? []).map((token) => ({
      id: token.id,
      name: token.name,
      prefix: token.prefix,
      scopes: token.scopes.map(tokenScopeLabel).join(', '),
      expiry: tokenExpiry(token, now),
      lastUsedAt: token.lastUsedAt,
      createdAt: token.createdAt,
    }))
  }, [query.data])

  return {
    state: loadState(query),
    rows,
    scopeOptions: tokenScopeOptions,
    formOpen,
    openForm: () => {
      create.reset()
      setSecret(null)
      setFormOpen(true)
    },
    closeForm: () => setFormOpen(false),
    register: form.register,
    errors: form.formState.errors,
    onSubmit: form.handleSubmit((values) => create.mutate(values)),
    isCreating: create.isPending,
    createError: create.error?.message ?? null,
    secret,
    dismissSecret: () => setSecret(null),
    revoke: (id: string) => revoke.mutate(id),
    revokingId: revoke.isPending ? (revoke.variables ?? null) : null,
    revokeError: revoke.error?.message ?? null,
  }
}

export type ApiTokensModel = ReturnType<typeof useApiTokens>
