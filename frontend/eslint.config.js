import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'

export default tseslint.config(
  { ignores: ['dist', '.pnp.cjs', '.pnp.loader.mjs', '.yarn'] },
  {
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    files: ['**/*.{ts,tsx}'],
    languageOptions: {
      ecmaVersion: 2022,
      globals: globals.browser,
    },
    plugins: {
      'react-hooks': reactHooks,
      'react-refresh': reactRefresh,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      'react-refresh/only-export-components': ['warn', { allowConstantExport: true }],
    },
  },
  {
    // Pages and components only render (docs/CODE_RULES.md § 4.2): data
    // access, mutations, derived state and effects live in use* hooks.
    files: ['src/app/**/*.tsx', 'src/components/**/*.tsx', 'src/features/**/*.tsx'],
    ignores: ['**/*.test.tsx'],
    rules: {
      '@typescript-eslint/no-restricted-imports': [
        'error',
        {
          paths: [
            {
              name: '@tanstack/react-query',
              allowTypeImports: true,
              message: 'Query and mutate in a use* hook, not in a component.',
            },
            {
              name: 'react',
              importNames: ['useMemo', 'useEffect', 'useLayoutEffect', 'useReducer'],
              message: 'Derived state and effects belong in a use* hook.',
            },
          ],
          patterns: [
            {
              group: ['**/lib/api/*'],
              allowTypeImports: true,
              message: 'Call the API from a use* hook, not from a component.',
            },
          ],
        },
      ],
    },
  },
)
