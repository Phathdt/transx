//  @ts-check

import { tanstackConfig } from '@tanstack/eslint-config'

export default [
  ...tanstackConfig,
  {
    rules: {
      'import/no-cycle': 'off',
      'import/order': 'off',
      'sort-imports': 'off',
      '@typescript-eslint/array-type': 'off',
      '@typescript-eslint/require-await': 'off',
      'pnpm/json-enforce-catalog': 'off',
    },
  },
  {
    ignores: [
      'eslint.config.js',
      'prettier.config.js',
      // Build output.
      'dist/**',
      // Generated artifacts: Orval API client and the router tree. Do not lint
      // or hand-edit; regenerate via `yarn generate:api` / `yarn generate-routes`.
      'src/lib/api/generated/**',
      'src/routeTree.gen.ts',
    ],
  },
]
