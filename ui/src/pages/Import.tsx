import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError } from '../api/client'
import {
  fetchImportJob,
  isImportJobActive,
  resolveImport,
  uploadImport,
  type ImportJob,
} from '../api/imports'

/** Poll cadence while the backend chews through the upload. */
const POLL_INTERVAL_MS = 1000

function uploadErrorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code === 'jobs_busy') {
      return 'A sync or import is already running — wait a moment, then try again.'
    }
    if (err.code === 'invalid_import_file') {
      return `That file was rejected: ${err.message}`
    }
    return err.message
  }
  return 'Upload failed. Check your connection and try again.'
}

function resolveErrorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code === 'import_not_finished') {
      return 'The import is still processing — wait for it to finish, then resolve again.'
    }
    return err.message
  }
  return 'Resolving titles failed. Try again.'
}

interface UnresolvedFormProps {
  jobId: number
  unresolved: string[]
}

/**
 * Manual resolution for titles TMDB could not match:
 * one TMDB show ID input per title; only filled-in rows are submitted.
 */
function UnresolvedForm({ jobId, unresolved }: UnresolvedFormProps) {
  const queryClient = useQueryClient()
  const [inputs, setInputs] = useState<Record<string, string>>({})
  const [validationError, setValidationError] = useState<string | null>(null)

  const mutation = useMutation({
    mutationFn: (mappings: Record<string, number>) => resolveImport(jobId, mappings),
    onSuccess: (job: ImportJob) => {
      setInputs({})
      queryClient.setQueryData(['importJob', jobId], job)
    },
  })

  const handleSubmit = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    const mappings: Record<string, number> = {}
    for (const [title, raw] of Object.entries(inputs)) {
      if (raw.trim() === '') continue
      const id = Number(raw)
      if (!Number.isInteger(id) || id <= 0) {
        setValidationError(`TMDB ID for "${title}" must be a positive whole number.`)
        return
      }
      mappings[title] = id
    }
    if (Object.keys(mappings).length === 0) {
      setValidationError('Enter a TMDB ID for at least one title.')
      return
    }
    setValidationError(null)
    mutation.mutate(mappings)
  }

  return (
    <form onSubmit={handleSubmit} aria-label="Resolve unmatched titles">
      <h3>Unmatched titles</h3>
      <p>
        These shows could not be matched automatically. Look each one up on TMDB and enter its
        show ID to finish importing it — or leave it blank to skip.
      </p>
      <ul className="list">
        {unresolved.map((title) => (
          <li key={title} className="card card-row wrap">
            <span className="grow">{title}</span>
            <input
              type="text"
              inputMode="numeric"
              placeholder="TMDB ID"
              aria-label={`TMDB ID for ${title}`}
              value={inputs[title] ?? ''}
              onChange={(e) => setInputs((prev) => ({ ...prev, [title]: e.target.value }))}
            />
          </li>
        ))}
      </ul>
      {validationError !== null && <p role="alert" className="alert">{validationError}</p>}
      {mutation.isError && <p role="alert" className="alert">{resolveErrorMessage(mutation.error)}</p>}
      <button type="submit" className="btn-primary" disabled={mutation.isPending}>
        {mutation.isPending ? 'Resolving…' : 'Resolve titles'}
      </button>
    </form>
  )
}

interface JobProgressProps {
  jobId: number
}

/** Polls job status while PENDING/RUNNING, then shows the outcome. */
function JobProgress({ jobId }: JobProgressProps) {
  const {
    data: job,
    isPending,
    isError,
  } = useQuery({
    queryKey: ['importJob', jobId],
    queryFn: () => fetchImportJob(jobId),
    refetchInterval: (query) => {
      const status = query.state.data?.status
      return status !== undefined && isImportJobActive(status) ? POLL_INTERVAL_MS : false
    },
  })

  if (isPending) {
    return <p className="muted">Checking import status…</p>
  }
  if (isError) {
    return <p role="alert" className="alert">Could not fetch import status. Try reloading.</p>
  }

  if (isImportJobActive(job.status)) {
    return (
      <div className="stack">
        <p>
          Importing… {job.processed} / {job.total} shows processed
          {job.skipped > 0 && ` (${job.skipped} malformed rows skipped)`}
        </p>
        <progress value={job.processed} max={Math.max(job.total, 1)} />
      </div>
    )
  }

  if (job.status === 'FAILED') {
    return (
      <p role="alert" className="alert">
        Import failed: {job.error !== undefined && job.error !== '' ? job.error : 'unknown error'}
      </p>
    )
  }

  // DONE
  return (
    <div>
      <p className="notice">
        Import complete: {job.processed} / {job.total} shows processed
        {job.skipped > 0 && `, ${job.skipped} malformed rows skipped`}
        {job.unresolved.length > 0 && `, ${job.unresolved.length} titles need manual resolution`}.
      </p>
      {job.unresolved.length > 0 ? (
        <UnresolvedForm jobId={jobId} unresolved={job.unresolved} />
      ) : (
        <p>Everything matched — you&apos;re all set.</p>
      )}
    </div>
  )
}

/**
 * TV Time import wizard: upload the export → poll job progress →
 * resolve any unmatched titles manually.
 */
export default function Import() {
  const [files, setFiles] = useState<File[]>([])
  const [jobId, setJobId] = useState<number | null>(null)

  const upload = useMutation({
    mutationFn: uploadImport,
    onSuccess: ({ jobId: created }) => setJobId(created),
  })

  const handleSubmit = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault()
    if (files.length === 0 || upload.isPending) return
    upload.mutate(files)
  }

  return (
    <section>
      <h1>Import</h1>
      <p>
        Import your TV Time history: upload <code>followed_shows.csv</code> and/or{' '}
        <code>seen_episodes.csv</code> — raw CSVs or the exported zip.
      </p>

      {jobId === null ? (
        <form className="stack" onSubmit={handleSubmit} style={{ maxWidth: '32rem' }}>
          <label className="field">
            <span>Export files</span>
            <input
              type="file"
              accept=".csv,.zip"
              multiple
              onChange={(e) => setFiles(Array.from(e.target.files ?? []))}
            />
          </label>
          <div>
            <button type="submit" className="btn-primary" disabled={files.length === 0 || upload.isPending}>
              {upload.isPending ? 'Uploading…' : 'Start import'}
            </button>
          </div>
          {upload.isError && <p role="alert" className="alert">{uploadErrorMessage(upload.error)}</p>}
        </form>
      ) : (
        <>
          <JobProgress jobId={jobId} />
          <button
            type="button"
            className="btn-ghost"
            onClick={() => {
              setJobId(null)
              setFiles([])
              upload.reset()
            }}
            style={{ marginTop: '1rem' }}
          >
            Import another file
          </button>
        </>
      )}
    </section>
  )
}
