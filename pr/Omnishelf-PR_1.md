# Feature Implementation Roadmap & Bug Fixes

## Overview

This PR outlines the upcoming feature additions, UX/UI improvements, and external integrations planned for OmniShelf. These updates focus on improving discovery, standardizing library views, and expanding the application's ecosystem.

## UI/UX Improvements & Formatting

- **Book Discovery Summaries:** The discover page for books will now display brief summaries/synopses within the card or upon hover/expansion, providing immediate context without requiring a full page load.
    
- **Transparent Season Backgrounds:** Implemented a transparent background effect when scrolling through seasons of a series, improving aesthetic immersion and blending better with the app's overall theme.
    
- **Discover Page Text Formatting:** Resolved poor text formatting on entry cards within the Discover page. Titles, metadata, and tags will now dynamically resize and clamp to prevent overflow and ensure a uniform, clean grid layout.
    
- **"Not Started" Section:**
    
    - Added a dedicated "Not Started Yet" section for TV shows.
        
    - Converted the existing "Plan to Watch" section to seamlessly integrate with the new "Not Started Yet" categorization.
        
- **Universal Summary Section:** Added a standardized summary section for all tracked media types (TV, Movies, Books, Games). This ensures consistent synopsis availability across the platform, including overviews for full TV Shows as well as individual TV Episodes.
    
- **Emotional Diary Logging:** Added an optional emotional reaction and quick-diary prompt immediately after marking an episode or individual piece of media as completed, capturing the immediate user sentiment similar to TV Time.
    

## Quality of Life & State Management

- **Default Import State for Movies:** Movies imported from external sources (e.g., TMDB, CSV imports) will now automatically be marked as "Plan to Watch" by default rather than defaulting to an active state.
    
- **Asynchronous Card Query Queue:** Implemented a non-blocking queue system for external API calls (such as fetching card data or metadata). Users can now navigate away to other pages while queries process in the background.
    

## Ecosystem & Integrations

- **Media Relations, Release Dates & Creator Deep-Dives:**
    
    - All tracked media (including Video Games) will now explicitly display their original release dates.
        
    - Added a "Related" section to media pages. This tab will surface other entries within the same series/franchise (e.g., listing all games in a specific video game series) as well as cross-media adaptations (e.g., linking a movie adaptation to its original book).
        
    - **Creator Entities:** Authors, Game Developers, and Publishers are now clickable entities. Users can click a creator to view a dedicated sub-page filtering the user's library by that specific creator.
        
- **"Time to Complete" Estimates:** Integrated the HowLongToBeat (HLTB) API to display estimated completion times for video games (Main Story vs. Completionist). Book completion times are dynamically estimated based on page count and the user's historical reading speed.
    
- **Google Calendar Integration:** Users can now sync their OmniShelf accounts with Google Calendar to automatically populate upcoming release dates for saved TV shows, movies, and expected book releases.
    
- **Kindle Auto-Updates:** Initiated integration research for Amazon Kindle to automatically update the user's book library and reading progress based on their Kindle syncing data.
    
- **Media Server Integration (Seerr):** Implemented integration hooks for the Seerr request management API to bridge OmniShelf tracking with local Plex, Jellyfin, or Emby streaming availability and automated media requests.
    

## Bug Fixes & Investigation

- **OpenLibrary 502 Errors:** Investigated and resolved issues regarding the OpenLibrary API returning 502 Unreachable errors resulting in poor book search performance. Implemented retry logic and secondary fallback APIs.
    
- **Recommendation Engine Optimization:** Added a robust recommendation algorithm that analyzes the user's completed items and highly-rated genres to intelligently suggest what to watch, read, or play next.