import { type FormEvent, useEffect, useState } from 'react'
import {
  Collection,
  getCollections,
  createCollection,
  deleteCollection,
  updateCollection
} from '../api/collections'

export default function Collections() {
  const [collections, setCollections] = useState<Collection[]>([])
  const [name, setName] = useState('')
  const [slug, setSlug] = useState('')
  const [error, setError] = useState<string>()
  const [loading, setLoading] = useState(true)

  // Edit state
  const [editingId, setEditingId] = useState<number | null>(null)
  const [editName, setEditName] = useState('')
  const [editSlug, setEditSlug] = useState('')

  const load = async () => {
    try {
      setLoading(true)
      const data = await getCollections()
      setCollections(data)
    } catch (err: any) {
      setError(err.message || 'Failed to load collections')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    load()
  }, [])

  const handleCreate = async (e: FormEvent) => {
    e.preventDefault()
    try {
      setError(undefined)
      await createCollection(name, slug)
      setName('')
      setSlug('')
      await load()
    } catch (err: any) {
      setError(err.message || 'Failed to create collection')
    }
  }

  const handleDelete = async (id: number) => {
    if (!window.confirm('Are you sure you want to delete this collection?')) return
    try {
      setError(undefined)
      await deleteCollection(id)
      await load()
    } catch (err: any) {
      setError(err.message || 'Failed to delete collection')
    }
  }

  const startEdit = (c: Collection) => {
    setEditingId(c.id)
    setEditName(c.name)
    setEditSlug(c.slug)
  }

  const cancelEdit = () => {
    setEditingId(null)
    setEditName('')
    setEditSlug('')
  }

  const handleUpdate = async (e: FormEvent) => {
    e.preventDefault()
    if (!editingId) return
    try {
      setError(undefined)
      await updateCollection(editingId, editName, editSlug)
      cancelEdit()
      await load()
    } catch (err: any) {
      setError(err.message || 'Failed to update collection')
    }
  }

  return (
    <section>
      <h1>Collections</h1>

      {error && (
        <div style={{ color: 'red', marginBottom: '1rem', padding: '0.5rem', border: '1px solid red' }}>
          {error}
        </div>
      )}

      <div className="card" style={{ marginBottom: '2rem' }}>
        <h2>Create Collection</h2>
        <form onSubmit={handleCreate} style={{ display: 'flex', gap: '1rem', alignItems: 'flex-end', flexWrap: 'wrap' }}>
          <div>
            <label htmlFor="name" style={{ display: 'block', marginBottom: '0.25rem' }}>Name</label>
            <input
              id="name"
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
            />
          </div>
          <div>
            <label htmlFor="slug" style={{ display: 'block', marginBottom: '0.25rem' }}>Slug</label>
            <input
              id="slug"
              type="text"
              value={slug}
              onChange={(e) => setSlug(e.target.value)}
              required
            />
          </div>
          <button type="submit" className="btn-primary">
            Create
          </button>
        </form>
      </div>

      <h2>Your Collections</h2>
      {loading ? (
        <p>Loading...</p>
      ) : collections.length === 0 ? (
        <p className="muted">You have no collections.</p>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: '1rem' }}>
          {collections.map((c) => (
            <div key={c.id} className="card" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              {editingId === c.id ? (
                <form onSubmit={handleUpdate} style={{ display: 'flex', gap: '1rem', alignItems: 'center', width: '100%' }}>
                  <input
                    type="text"
                    value={editName}
                    onChange={(e) => setEditName(e.target.value)}
                    required
                    placeholder="Name"
                  />
                  <input
                    type="text"
                    value={editSlug}
                    onChange={(e) => setEditSlug(e.target.value)}
                    required
                    placeholder="Slug"
                  />
                  <div style={{ marginLeft: 'auto', display: 'flex', gap: '0.5rem' }}>
                    <button type="submit" className="btn-primary">Save</button>
                    <button type="button" onClick={cancelEdit}>Cancel</button>
                  </div>
                </form>
              ) : (
                <>
                  <div>
                    <h3 style={{ margin: '0 0 0.25rem 0' }}>{c.name}</h3>
                    <span className="muted" style={{ fontSize: '0.9rem' }}>Slug: {c.slug}</span>
                  </div>
                  <div style={{ display: 'flex', gap: '0.5rem' }}>
                    <button type="button" onClick={() => startEdit(c)}>
                      Edit
                    </button>
                    <button
                      type="button"
                      onClick={() => handleDelete(c.id)}
                      style={{ color: 'red' }}
                    >
                      Delete
                    </button>
                  </div>
                </>
              )}
            </div>
          ))}
        </div>
      )}
    </section>
  )
}
