import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { useNavigate } from 'react-router-dom'
import { sessionQueryKey } from '../../hooks/useSession'
import { ApiError, changePassword } from '../../lib/api/auth'
import { passwordSchema, type PasswordValues } from '../../schemas/password'

/**
 * Password change. The Manager revokes every session of the user, including
 * this one, so a success returns to the login page with a notice.
 */
export function useChangePassword() {
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const form = useForm<PasswordValues>({
    resolver: zodResolver(passwordSchema),
    defaultValues: { currentPassword: '', newPassword: '', confirmPassword: '' },
  })
  const mutation = useMutation({
    mutationFn: (values: PasswordValues) =>
      changePassword(values.currentPassword, values.newPassword),
    onSuccess: () => {
      form.reset()
      queryClient.removeQueries({ queryKey: sessionQueryKey })
      navigate('/login', {
        replace: true,
        state: { notice: 'Mot de passe modifié. Reconnectez-vous avec le nouveau mot de passe.' },
      })
    },
  })
  const error = mutation.error
  return {
    register: form.register,
    errors: form.formState.errors,
    onSubmit: form.handleSubmit((values) => mutation.mutate(values)),
    isPending: mutation.isPending,
    errorMessage:
      error instanceof ApiError && error.status === 401
        ? 'Le mot de passe actuel est incorrect.'
        : (error?.message ?? null),
  }
}
