import js from '@eslint/js';
import globals from 'globals';

export default [
    {
        ignores: [
            'coverage/**',
            'node_modules/**',
            'public/js/vendor/**',
        ],
    },
    js.configs.recommended,
    {
        files: ['**/*.js'],
        languageOptions: {
            ecmaVersion: 'latest',
            sourceType: 'module',
            globals: globals.node,
        },
        rules: {
            'no-constant-condition': 'off',
            'no-empty': ['error', { allowEmptyCatch: true }],
            'no-unused-vars': 'error',
            'no-useless-assignment': 'off',
            'preserve-caught-error': 'off',
        },
    },
    {
        files: ['public/js/**/*.js'],
        languageOptions: {
            globals: {
                ...globals.browser,
                mermaid: 'readonly',
            },
        },
    },
    {
        files: ['test/**/*.js'],
        languageOptions: {
            globals: {
                ...globals.browser,
                ...globals.vitest,
            },
        },
    },
];
