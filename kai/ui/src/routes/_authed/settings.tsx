import { createRoute } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { authedRoute } from '../_authed'
import { apiFetch } from '../../api/client'
import { useMe } from '../../hooks/useMe'
import type { APIKey } from '../../api/types'

export const settingsRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: '/settings',
  component: SettingsPage,
})

interface CreateKeyResponse {
  id: string
  name: string
  key: string
  key_prefix: string
}

function SettingsPage() {
  const { data: me } = useMe()
  const qc = useQueryClient()
  const [newKeyName, setNewKeyName] = useState('')
  const [revealedKey, setRevealedKey] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)

  const { data: keys, isLoading: keysLoading } = useQuery({
    queryKey: ['api-keys'] as const,
    queryFn: () => apiFetch<APIKey[]>('/keys'),
  })

  const createMutation = useMutation({
    mutationFn: (name: string) =>
      apiFetch<CreateKeyResponse>('/keys', {
        method: 'POST',
        body: JSON.stringify({ name }),
      }),
    onSuccess: (data) => {
      setRevealedKey(data.key)
      setNewKeyName('')
      void qc.invalidateQueries({ queryKey: ['api-keys'] })
    },
  })

  const revokeMutation = useMutation({
    mutationFn: (keyId: string) =>
      apiFetch<Record<string, unknown>>(`/keys/${keyId}`, { method: 'DELETE' }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['api-keys'] })
    },
  })

  function handleRevoke(keyId: string, keyName: string) {
    if (window.confirm(`Revoke API key "${keyName}"? This cannot be undone.`)) {
      revokeMutation.mutate(keyId)
    }
  }

  function handleCopy() {
    if (!revealedKey) return
    void navigator.clipboard.writeText(revealedKey)
    setCopied(true)
    setTimeout(() => setCopied(false), 2_000)
  }

  return (
    <div className="h-full overflow-y-auto">
      <div className="max-w-2xl mx-auto px-6 py-8 space-y-10">
        <h1 className="text-2xl font-bold text-white">Settings</h1>

        {/* ── Profile ─────────────────────────────────────────────── */}
        <section className="space-y-4">
          <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider border-b border-gray-800 pb-2">
            Profile
          </h2>
          {me ? (
            <div className="rounded-lg border border-gray-800 bg-gray-900 p-4 space-y-3">
              <div>
                <p className="text-xs text-gray-500 mb-1">Display Name</p>
                <p className="text-sm text-gray-200">{me.display_name}</p>
              </div>
              <div>
                <p className="text-xs text-gray-500 mb-1">Email</p>
                <p className="text-sm text-gray-200">{me.email}</p>
              </div>
            </div>
          ) : (
            <div className="h-20 bg-gray-800 rounded-lg animate-pulse" />
          )}
        </section>

        {/* ── API Keys ─────────────────────────────────────────────── */}
        <section className="space-y-4">
          <h2 className="text-sm font-semibold text-gray-400 uppercase tracking-wider border-b border-gray-800 pb-2">
            API Keys
          </h2>

          {/* One-time key reveal box */}
          {revealedKey && (
            <div className="rounded-lg border border-emerald-700 bg-emerald-900/20 p-4 space-y-3">
              <p className="text-xs font-semibold text-emerald-300">
                ⚠ This key will not be shown again. Copy it now.
              </p>
              <div className="flex items-center gap-2">
                <code className="flex-1 text-xs font-mono text-emerald-200 bg-gray-950 px-3 py-2 rounded border border-gray-800 truncate select-all">
                  {revealedKey}
                </code>
                <button
                  onClick={handleCopy}
                  className="flex-shrink-0 rounded border border-emerald-700 bg-emerald-900/30 px-3 py-2 text-xs text-emerald-300 hover:bg-emerald-900/60 transition-colors"
                >
                  {copied ? '✓ Copied' : 'Copy'}
                </button>
              </div>
              <button
                onClick={() => setRevealedKey(null)}
                className="text-xs text-gray-500 hover:text-gray-400 transition-colors"
              >
                Dismiss
              </button>
            </div>
          )}

          {/* Create key form */}
          <form
            onSubmit={(e) => {
              e.preventDefault()
              if (newKeyName.trim()) createMutation.mutate(newKeyName.trim())
            }}
            className="flex items-center gap-2"
          >
            <input
              type="text"
              value={newKeyName}
              onChange={(e) => setNewKeyName(e.target.value)}
              placeholder="Key name…"
              className="flex-1 rounded-md border border-gray-700 bg-gray-900 px-3 py-2 text-sm text-gray-200 placeholder-gray-600 focus:border-indigo-500 focus:outline-none"
            />
            <button
              type="submit"
              disabled={!newKeyName.trim() || createMutation.isPending}
              className="rounded-md border border-indigo-700 bg-indigo-900/30 px-4 py-2 text-sm font-medium text-indigo-300 hover:bg-indigo-900/60 disabled:opacity-50 transition-colors"
            >
              {createMutation.isPending ? 'Creating…' : 'Create API Key'}
            </button>
          </form>
          {createMutation.isError && (
            <p className="text-xs text-rose-400">Failed to create key. Try again.</p>
          )}

          {/* Keys table */}
          {keysLoading ? (
            <div className="space-y-2">
              {[1, 2].map((i) => (
                <div key={i} className="h-12 bg-gray-800 rounded animate-pulse" />
              ))}
            </div>
          ) : !keys || keys.length === 0 ? (
            <p className="text-sm text-gray-500 italic py-4">No API keys yet.</p>
          ) : (
            <div className="rounded-lg border border-gray-800 overflow-hidden">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-gray-800 bg-gray-900/50">
                    <th className="px-4 py-2.5 text-left text-xs font-semibold text-gray-500">
                      Prefix
                    </th>
                    <th className="px-4 py-2.5 text-left text-xs font-semibold text-gray-500">
                      Name
                    </th>
                    <th className="px-4 py-2.5 text-left text-xs font-semibold text-gray-500">
                      Created
                    </th>
                    <th className="px-4 py-2.5 text-left text-xs font-semibold text-gray-500">
                      Last Used
                    </th>
                    <th className="px-4 py-2.5" />
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-800">
                  {keys.map((key) => (
                    <tr
                      key={key.id}
                      className="bg-gray-900 hover:bg-gray-900/80 transition-colors"
                    >
                      <td className="px-4 py-3 font-mono text-xs text-gray-400">
                        {key.key_prefix}…
                      </td>
                      <td className="px-4 py-3 text-gray-200">{key.name}</td>
                      <td className="px-4 py-3 text-xs text-gray-500">
                        {new Date(key.created_at).toLocaleDateString()}
                      </td>
                      <td className="px-4 py-3 text-xs text-gray-500">
                        {key.last_used_at
                          ? new Date(key.last_used_at).toLocaleDateString()
                          : '—'}
                      </td>
                      <td className="px-4 py-3 text-right">
                        <button
                          onClick={() => handleRevoke(key.id, key.name)}
                          disabled={revokeMutation.isPending}
                          className="text-xs text-rose-400 hover:text-rose-300 disabled:opacity-50 transition-colors"
                        >
                          Revoke
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>
      </div>
    </div>
  )
}
