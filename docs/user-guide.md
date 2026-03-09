# AdHive User Guide

**Version:** 1.0.0  
**Last Updated:** 2026-03-09

---

## Table of Contents

1. [Introduction](#introduction)
2. [Getting Started](#getting-started)
3. [Managing Entries](#managing-entries)
4. [Searching & Filtering](#searching--filtering)
5. [Tags & Organization](#tags--organization)
6. [Interactions & Notes](#interactions--notes)
7. [Archive Management](#archive-management)
8. [Troubleshooting](#troubleshooting)

---

## Introduction

AdHive is a self-hosted catalog application for organizing and reviewing archived classified advertisements. It helps you catalog, search, and interact with archived ads from various sources.

### Key Features

- 📁 **Catalog Management** - Organize ads with tags, notes, and custom fields
- 🔍 **Full-Text Search** - Fast search across titles, descriptions, and notes
- 🎲 **Random Review** - Discover forgotten ads with weighted random selection
- 🖼️ **Thumbnail Management** - Automatic thumbnail extraction
- 📦 **Archive Storage** - Store multiple revisions of archived pages

---

## Getting Started

### First-Time Setup

1. **Access AdHive** - Open your browser and navigate to your AdHive installation URL
2. **Create Account** - Click "Register" and enter your email and password
3. **Log In** - Use your credentials to sign in

### The Dashboard

After logging in, you'll see the main entries list with:

- **Sidebar** - Navigation and filters
- **Search Bar** - Quick search across all entries
- **Entry Grid/List** - Display of your cataloged ads
- **Action Bar** - Bulk operations and sorting

---

## Managing Entries

### Adding New Entries

1. Click the **+ Add Entry** button (or press `N`)
2. Enter the URL of the ad page you want to archive
3. Click **Add** - AdHive will fetch and archive the page
4. The entry appears in your catalog with auto-extracted title and thumbnail

### Viewing Entry Details

Click on any entry card to view:
- Full description and metadata
- All archive revisions
- Interaction history (notes, scores, contact status)

### Editing Entries

1. Open the entry detail page
2. Click **Edit** to modify:
   - Title
   - Description
   - Phone number
   - Location
   - Archive fidelity (high/partial/low)

### Deleting Entries

1. Select entries using checkboxes
2. Click **Delete** in the action bar
3. Confirm deletion

> ⚠️ **Note:** Deletion is permanent. Archived files are also removed.

---

## Searching & Filtering

### Quick Search

Use the search bar at the top to find entries by:
- Title
- Description
- Notes
- Tags

### Advanced Filters

Click **Filter** to access:
- **By Tag** - Filter by one or more tags
- **By Status** - Archive status (pending, archived, failed)
- **By Source** - Domain the entry came from
- **By Location** - Geographic location
- **By Date** - Created/updated date range
- **Has Interaction** - Entries you've reviewed

### Sorting

Sort entries by:
- Created date
- Updated date
- Title (A-Z)

### Random Selection

Click **Pick Random** to get a random entry for review. You can:
- Filter to untried entries only
- Set minimum score requirements
- Include/exclude specific tags

---

## Tags & Organization

### Creating Tags

1. Go to the **Tags** page
2. Click **+ New Tag**
3. Enter a name and choose a color
4. Click **Create**

### Managing Tags

- **Edit** - Change name or color
- **Delete** - Remove tag (prompts to remove from entries)
- **Filter** - Click a tag to see all entries with it

### Bulk Tagging

1. Select multiple entries
2. Click **Tag** in the action bar
3. Choose tags to add or remove

---

## Interactions & Notes

### Recording Interactions

For each entry, you can track:

| Field | Description |
|-------|-------------|
| **Tried** | Whether you've attempted contact |
| **Score** | Interest level (1-5 stars) |
| **Comments** | Your notes about this ad |
| **Contacted At** | Date of first contact |
| **Purchased At** | Date of purchase (if applicable) |

### Updating Interaction

1. Open an entry detail page
2. Scroll to the **Interaction** section
3. Click **Add Interaction** or edit existing
4. Save your updates

### Filtering by Interaction

Use the **Has Interaction** filter to find:
- Entries you've reviewed
- Entries you haven't tried yet
- High-scored entries

---

## Archive Management

### Understanding Archives

Each entry stores:
- The original URL
- Multiple revision snapshots
- Thumbnail images

### Viewing Archive Revisions

1. Open an entry detail page
2. Go to **Archive Revisions**
3. Click any revision to view that snapshot
4. Use timeline to navigate between versions

### Refreshing Archives

To update an archive with the latest version:

1. Open the entry detail page
2. Click **Refresh Archive**
3. AdHive fetches the current page state

### Archive Status

| Status | Meaning |
|--------|---------|
| Pending | Awaiting initial archive |
| Archived | Successfully stored |
| Failed | Archive attempt failed |
| Partial | Only some revisions stored |

---

## Troubleshooting

### Common Issues

**Can't log in**
- Verify your credentials
- Check session hasn't expired
- Clear browser cookies and try again

**Entry won't add**
- Verify the URL is accessible
- Some sites block automated access
- Check if the domain is in your allowed sources

**Archive fails**
- Site may be down or blocking requests
- Try refreshing later
- Check the archive status for error details

**Search not finding results**
- Try simpler search terms
- Check filters aren't too restrictive
- Some fields may not be indexed yet

### Getting Help

If you encounter issues not covered here:

1. Check the [API Documentation](../api.md)
2. Review the [Architecture Document](../architecture.md)
3. Check server logs for error details
4. Report bugs via your support channels

---

## Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| `N` | New entry |
| `/` | Focus search |
| `Esc` | Close modals |
| `Enter` | Submit forms |

---

## Security Notes

- Always use a strong password
- Session cookies are secure (HttpOnly, Secure)
- CSRF protection is enabled for all mutations
- Clear your session when using shared devices

---

*This guide covers the basics of using AdHive. For deployment and technical details, see the Deployment Guide.*