# Phase 3: Self-Sovereign Automation, Physical Collection Analytics & Offline Discovery

## Overview

This PR outlines the Phase 3 feature roadmap for OmniShelf. Building upon the Phase 2 foundation of **Genuine Ownership**, Phase 3 doubles down on self-sovereign media automation, local media server watch state synchronization, physical trading card market value tracking, cross-media journaling, and offline-first discovery.

> **Ownership Philosophy Note:** Proprietary digital platforms reliant on restrictive Digital Rights Management (DRM) or leased licensing—such as **Steam**, **Epic Games Store**, or proprietary cloud platforms—are intentionally excluded from API synchronization. OmniShelf is strictly architected for products owned **physically** (physical game cartridges/discs, print books, physical trading cards) and self-hosted, **DRM-free** media assets (GOG downloads, local NAS ePubs/audiobooks/FLACs, and self-hosted Jellyfin/Plex servers).

---

## 1. Self-Hosted & DRM-Free Media Server Automation

- **Jellyfin / Emby / Plex Outbound Webhook Receiver:**
  - Implements an inbound webhook endpoint (`POST /api/webhooks/mediaserver`).
  - When media is played or completed on a self-hosted Jellyfin, Emby, or Plex instance, the webhook automatically parses TMDB metadata and updates episode or movie watch states in OmniShelf in real-time.
- **Audiobookshelf & Calibre Progress Synchronization:**
  - Integrates automated progress syncing with self-hosted Audiobookshelf and Calibre servers.
  - Automatically updates page counts, listening progress percentages, and finish dates for DRM-free eBooks and audiobooks stored on local NAS storage without relying on external cloud APIs.
- **Local NAS Storage Mapping & File Inspection `[Future Feature]`:**
  - *Status: Designated for Future Phase / Backlog Integration.*
  - Scans mounted local filesystem directories on TrueNAS / Unraid volumes (`/media`, `/epubs`, `/roms`), displaying exact file paths, container formats, audio/video codecs, and file sizes directly on media detail cards to verify physical file presence.

---

## 2. Physical Collection & Trading Card Analytics

- **Trading Card Market Value History & Price Delta Graphs:**
  - Integrates interactive historical price line graphs into Pokémon TCG and Yu-Gi-Oh! card detail pages using periodic price snapshots (TCGPlayer / Cardmarket).
  - Displays total physical binder portfolio value and individual card value fluctuations over time.
- **Multi-Slot Binder Camera OCR Scanner:**
  - Upgrades the mobile scanner to parse multi-card binder pages (e.g. 9-pocket trading card sheets) in a single camera snapshot for rapid physical cataloging.
- **Physical Storage Location Tagging:**
  - Adds physical location metadata fields to all media items (`Shelf A - Living Room`, `Binder #3 - Retro Cards`, `Storage Box #2`), bridging digital inventory management with physical room organization.

---

## 3. Cross-Media Journaling & Emotional Logging

- **Universal Emotional Reaction & Journaling Engine:**
  - Extends emotional sentiment logging (*"Mind-blowing"*, *"Cozy"*, *"Tear-jerker"*, *"Instant Classic"*) and private journal logs across Movies, TV Shows, Games, Books, and Music Albums.
- **Re-watch & Re-read Timeline Tracking:**
  - Supports logging multiple completion instances for the same item over time, tracking date histories, rating shifts, and evolving journal thoughts across repeat viewings or readings.

---

## 4. Offline Discovery & Cross-Media Relationships

- **Franchise Adaptation & Source Material Linking:**
  - Automatically establishes bi-directional relationships between physical source books and DRM-free video game or film adaptations in the user's library (e.g., linking a physical novel to its self-hosted film adaptation).
- **Offline Natural Language Vibe Filter:**
  - Introduces a local "Vibe Filter" engine (*"Cozy autumn reads under 300 pages"*, *"High-octane 90s action films"*), executing queries against indexed local tags and metadata without transmitting user data to third-party analytics servers.

---

## 5. Power-User UX & Data Portability

- **Global Command Palette (`Ctrl + K` / `Cmd + K`):**
  - Implements a spotlight search overlay accessible anywhere in the application.
  - Supports rapid multi-media search, barcode scanner activation, collection jumps, and quick-status updates via hotkeys.
- **Self-Sovereign Data Backup & Portability:**
  - **1-Click Complete Data Export:** Generates full JSON/CSV backups of all tracked items, watch histories, custom tags, and private notes.
- **Cryptographically Signed Public Showcase:**
  - Generates secure, read-only public sharing tokens for user profiles, allowing users to showcase their current-month heatmap, stats, and badges without opening full database access.
- **Custom Theme System:**
  - Adds manual theme selection in Settings: *Dark Espresso* (default), *Midnight OLED*, and *Cream Paper*.
