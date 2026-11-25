import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { loadState } from '../../hooks/loadState'
import { getSecurityPolicy, updateSecurityPolicy } from '../../lib/api/security'
import {
  networksFromText,
  securityPolicySchema,
  type SecurityPolicyValues,
} from '../../schemas/securityPolicy'

export const securityPolicyQueryKey = ['security', 'policy'] as const

/** Logic and view model of the policy tab of UsersPage. */
export function useSecurityPolicyPanel() {
  const queryClient = useQueryClient()
  const policy = useQuery({ queryKey: securityPolicyQueryKey, queryFn: getSecurityPolicy })
  const form = useForm<SecurityPolicyValues>({
    resolver: zodResolver(securityPolicySchema),
    defaultValues: {
      idleTimeoutMinutes: 60,
      networks: '',
      requireAdminSecondFactor: false,
      allowPasskeyLogin: false,
    },
    values: policy.data
      ? {
          idleTimeoutMinutes: policy.data.idleTimeoutMinutes,
          networks: policy.data.allowedNetworks.join('\n'),
          requireAdminSecondFactor: policy.data.requireAdminSecondFactor ?? false,
          allowPasskeyLogin: policy.data.allowPasskeyLogin ?? false,
        }
      : undefined,
  })
  const mutation = useMutation({
    mutationFn: (values: SecurityPolicyValues) =>
      updateSecurityPolicy({
        idleTimeoutMinutes: values.idleTimeoutMinutes,
        allowedNetworks: networksFromText(values.networks),
        requireAdminSecondFactor: values.requireAdminSecondFactor,
        allowPasskeyLogin: values.allowPasskeyLogin,
      }),
    onSuccess: (updated) => queryClient.setQueryData(securityPolicyQueryKey, updated),
  })

  return {
    state: loadState(policy),
    loaded: policy.data != null,
    currentAddress: policy.data?.currentAddress ?? null,
    register: form.register,
    errors: form.formState.errors,
    onSubmit: form.handleSubmit((values) => mutation.mutate(values)),
    isPending: mutation.isPending,
    saved: mutation.isSuccess && !form.formState.isDirty,
    errorMessage: mutation.error?.message ?? null,
  }
}
