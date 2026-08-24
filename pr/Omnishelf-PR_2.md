# Phase 2: Genuine Ownership, Analytics & Automation

## Overview

This PR outlines the Phase 2 feature roadmap for OmniShelf. With the core tracking and metadata ingestion engines stabilized, this update shifts focus toward power-user organization, deep analytics, and codifying the application's core philosophy: **Genuine Ownership**. Moving forward, OmniShelf enforces distinct tracking paradigms to reflect true digital sovereignty and physical collections.

## Genuine Ownership & Ingestion

- **Video Games (Strictly Physical & DRM-Free):**
    
    - **Physical Barcode Scanning:** Extended the mobile barcode scanner to support modern and retro physical game cases.
        
    - **GOG API Integration:** Implemented an automatic library sync with GOG (Good Old Games) to track genuinely owned, DRM-free digital games. _(Note: Platforms reliant on restrictive DRM leasing, such as Steam or Epic Games, are intentionally excluded from API sync)._
        
- **Trading Cards (Strictly Physical):**
    
    - Enhanced the existing OCR pipeline to support bulk-scanning of physical trading cards for faster binder cataloging.
        
    - **Market Value History:** Integrated a historical line graph onto trading card detail pages, visualizing the card's market price delta over time based on periodic pricing API snapshots.
        

## Digital Sovereignty (TV, Movies & Books)

_Digital tracking remains supported for these mediums, heavily leaning on the local NAS environment._

- **The** _**Arr**_ **Stack Webhooks (Radarr/Sonarr):**
    
    - Implemented outbound webhook triggers. Marking a movie or TV show as "Plan to Watch" in OmniShelf will now optionally fire a request to Radarr/Sonarr to automate the download pipeline to the local NAS.
        
- **Local NAS File Mapping:**
    
    - Added an optional background scanner that monitors specific local TrueNAS directories (e.g., `/epubs`).
        
    - Automatically links detected local, DRM-free files to their corresponding database entries in OmniShelf, allowing users to know exactly what they physically hold on their drives.
        

## Organization & Library Management

- **Custom Tagging & Collections:**
    
    - Implemented a custom tagging system allowing users to create cross-media collections (e.g., "Cozy Sci-Fi," "Game Night," "Halloween Watchlist").
        
- **Advanced Multi-Criteria Filtering:**
    
    - Enhanced the filter dropdown UI to support inline searching, allowing users to quickly find and toggle specific tags or genres without endless scrolling.
        
    - Added the ability to filter the library based on user Star Ratings.
        
    - Added the ability to filter unplayed/unread media by **Time to Complete** (e.g., "Show me unplayed games under 10 hours" or "Books under 300 pages").
        
    - Overhauled the library view to support compound querying (e.g., filtering by `Status: Not Started` + `Star Rating: 4+` + `Media Type: Book`).
        
- **Bulk Actions Dashboard:**
    
    - Added a selection state to library cards, enabling users to bulk-update statuses, apply tags, or delete multiple entries simultaneously.
        

## System Maintenance & Data Integrity

- **Metadata Refresh CLI:**
    
    - Added a robust backend terminal command designed to run directly on the NAS host to refresh the database.
        
    - The script traverses the entire SQLite database and queries external APIs (TMDB, OpenLibrary, etc.) to fetch the latest metadata, replacing broken cover art, updating summaries, appending franchise relations, and syncing new tags.
        
    - **Safe-Update Mechanism:** Strictly isolates and preserves user-generated data. Watch status, current progress, custom tags, and star ratings remain entirely untouched during the refresh.
        

## Analytics & Gamification

- **"Time Spent" Dashboard:**
    
    - Added a dedicated statistics page calculating the total time the user has spent consuming media, broken down by days/hours/minutes across all supported media types.
        
- **Consumption Heatmap:**
    
    - Integrated a GitHub-style calendar heatmap on the user profile to visualize daily media logging habits and progress streaks.
        
- **Franchise Completion Badges:**
    
    - Added permanent profile badges awarded automatically when a user achieves 100% completion of a designated franchise or universe (e.g., completing all games in the _Zelda_ series, or reading all _Harry Potter_ books).
        

## Data Portability & Social

- **Public Read-Only Shelves:**
    
    - Users can now generate secure, read-only external links to specific custom tags or collections to share physical collection highlights or recommendations with friends without exposing their entire database.