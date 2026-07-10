import { scanBook, trackBook } from '../api/books'
import type { BookStatus } from '../api/books'
import { scanGame, trackGame } from '../api/games'
import type { GameStatus } from '../api/games'
import { scanAlbum, trackAlbum } from '../api/music'
import type { MusicStatus } from '../api/music'
import type { ScanTarget } from '../components/books/BulkScanner'

/** Books: ISBN → OpenLibrary; subtitle is the author list. */
export const bookScanTarget: ScanTarget<BookStatus> = {
  noun: 'book',
  codeNoun: 'ISBN',
  inputLabel: 'Scan ISBN',
  placeholder: 'Scan or type an ISBN, then Enter',
  statuses: [
    { value: 'READING', label: 'Reading' },
    { value: 'PLAN_TO', label: 'Plan to read' },
    { value: 'COMPLETED', label: 'Completed' },
  ],
  defaultStatus: 'READING',
  scan: async (code) => {
    const b = await scanBook(code)
    return { id: b.id, title: b.title, subtitle: b.authors, coverPath: b.coverPath }
  },
  track: (id, status) => trackBook(id, status),
}

/** Games: UPC/EAN → ScanDex; subtitle is the platform. */
export const gameScanTarget: ScanTarget<GameStatus> = {
  noun: 'game',
  codeNoun: 'barcode',
  inputLabel: 'Scan game barcode',
  placeholder: 'Scan or type a barcode, then Enter',
  statuses: [
    { value: 'PLAYING', label: 'Playing' },
    { value: 'PLAN_TO', label: 'Plan to play' },
    { value: 'COMPLETED', label: 'Completed' },
    { value: 'STOPPED', label: 'Stopped playing' },
  ],
  defaultStatus: 'PLAYING',
  scan: async (code) => {
    const g = await scanGame(code)
    return { id: g.id, title: g.title, subtitle: g.platform, coverPath: g.coverPath }
  },
  track: (id, status) => trackGame(id, status),
}

/** Music: UPC/EAN → Discogs; subtitle is the artist. */
export const musicScanTarget: ScanTarget<MusicStatus> = {
  noun: 'album',
  codeNoun: 'barcode',
  inputLabel: 'Scan album barcode',
  placeholder: 'Scan or type a barcode, then Enter',
  statuses: [
    { value: 'LISTENING', label: 'Listening' },
    { value: 'PLAN_TO', label: 'Plan to listen' },
    { value: 'COMPLETED', label: 'Listened' },
    { value: 'STOPPED', label: 'Set aside' },
  ],
  defaultStatus: 'LISTENING',
  scan: async (code) => {
    const a = await scanAlbum(code)
    return { id: a.id, title: a.title, subtitle: a.artist, coverPath: a.coverPath }
  },
  track: (id, status) => trackAlbum(id, status),
}
