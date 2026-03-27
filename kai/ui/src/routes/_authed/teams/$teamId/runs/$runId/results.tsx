import { createRoute, Link } from '@tanstack/react-router'
import { useQuery, useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { authedRoute } from '../../../../../_authed'
import { apiFetch } from '../../../../../../api/client'
import type { Artifact } from '../../../../../../api/types'

export const resultsRoute = createRoute({
  getParentRoute: () => authedRoute,
  path: '/teams/$teamId/runs/$runId/results',
  component: ResultsView,
})

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function ArtifactCard({
  artifact,
  onDownload,
  downloading,
}: {
  artifact: Artifact
  onDownload: () => void
  downloading: boolean
}) {
  return (
    <div className="rounded-lg border border-gray-800 bg-gray-900 p-4 flex flex-col gap-3">
      <div className="flex-1 min-w-0 space-y-1">
        <p
          className="text-sm font-medium text-gray-100 truncate"
          title={artifact.name}
        >
          {artifact.name}
        </p>
        <p className="text-xs text-gray-500">{artifact.mime_type}</p>
        <p className="text-xs text-gray-500">{formatSize(artifact.size_bytes)}</p>
        <p className="text-xs text-gray-600">
          {new Date(artifact.created_at).toLocaleString()}
        </p>
      </div>
      <button
        onClick={onDownload}
        disabled={downloading}
        className="w-full rounded-md border border-indigo-700 bg-indigo-900/30 px-3 py-1.5 text-xs font-medium text-indigo-300 hover:bg-indigo-900/60 disabled:opacity-50 transition-colors"
      >
        {downloading ? 'Requesting…' : '⬇ Download'}
      </button>
    </div>
  )
}

function ResultsView() {
  const { teamId, runId } = resultsRoute.useParams()
  const [toast, setToast] = useState<string | null>(null)

  const { data: artifacts, isLoading, error } = useQuery({
    queryKey: ['artifacts', runId] as const,
    queryFn: () => apiFetch<Artifact[]>(`/runs/${runId}/artifacts`),
  })

  const downloadMutation = useMutation({
    mutationFn: (artifactId: string) =>
      apiFetch<{ message?: string }>(`/artifacts/${artifactId}/download`),
    onSuccess: (data) => {
      if (data.message) {
        setToast(data.message)
        setTimeout(() => setToast(null), 4_000)
      }
    },
  })

  return (
    <div className="h-full overflow-y-auto">
      <div className="max-w-4xl mx-auto px-6 py-8 space-y-6">
        {/* Header */}
        <div className="flex items-start justify-between gap-4">
          <div>
            <nav className="flex items-center gap-2 text-xs text-gray-500 mb-2">
              <Link to="/" className="hover:text-gray-300 transition-colors">
                Dashboard
              </Link>
              <span>/</span>
              <Link
                to="/teams/$teamId"
                params={{ teamId }}
                className="hover:text-gray-300 transition-colors"
              >
                Team
              </Link>
              <span>/</span>
              <Link
                to="/teams/$teamId/runs/$runId"
                params={{ teamId, runId }}
                className="hover:text-gray-300 transition-colors"
              >
                {runId.slice(0, 8)}…
              </Link>
              <span>/</span>
              <span className="text-gray-400">Results</span>
            </nav>
            <h1 className="text-2xl font-bold text-white">Run Results</h1>
          </div>
          <Link
            to="/teams/$teamId/runs/$runId"
            params={{ teamId, runId }}
            className="flex-shrink-0 text-sm text-indigo-400 hover:text-indigo-300 transition-colors mt-6"
          >
            ← Back to Run
          </Link>
        </div>

        {/* Toast notification */}
        {toast && (
          <div className="rounded-lg border border-yellow-700 bg-yellow-900/20 px-4 py-3 text-sm text-yellow-300">
            {toast}
          </div>
        )}

        {/* Content */}
        {isLoading ? (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
            {[1, 2, 3].map((i) => (
              <div key={i} className="h-36 bg-gray-800 rounded-lg animate-pulse" />
            ))}
          </div>
        ) : error ? (
          <div className="rounded-lg border border-rose-800 bg-rose-900/20 px-4 py-3 text-sm text-rose-300">
            Failed to load artifacts. Check your connection and try again.
          </div>
        ) : !artifacts || artifacts.length === 0 ? (
          <div className="rounded-lg border border-gray-800 bg-gray-900 px-6 py-12 text-center">
            <p className="text-gray-400">No artifacts yet</p>
            <p className="text-sm text-gray-500 mt-1">
              Artifacts will appear here once the run produces output.
            </p>
          </div>
        ) : (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
            {artifacts.map((artifact) => (
              <ArtifactCard
                key={artifact.id}
                artifact={artifact}
                onDownload={() => downloadMutation.mutate(artifact.id)}
                downloading={
                  downloadMutation.isPending &&
                  downloadMutation.variables === artifact.id
                }
              />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
