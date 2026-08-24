/* Migration: add theme column to users table */
ALTER TABLE users ADD COLUMN theme TEXT NOT NULL DEFAULT 'light';
