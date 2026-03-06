import { writable } from 'svelte/store';

// Base API URL - would typically come from env
const API_BASE = '/api/v1';

export interface Entry {
  id: string;
  user_id: string;
  url: string;
  title: string;
  description: string;
  phone_number: string;
  location: string;
  thumbnail_path: string;
  archive_path: string;
  archive_status: string;
  archive_fidelity?: 'high' | 'partial' | 'low';
  archive_current_revision_id?: string;
  thumbnail_source?: 'auto' | 'user_selected' | 'upload';
  created_at: string;
  updated_at: string;
  tags?: Tag[];
}

export interface ArchiveRevision {
  id: string;
  entry_id: string;
  revision_no: number;
  engine: string;
  status: 'success' | 'partial' | 'failed';
  failure_reason?: string;
  captured_at: string;
}

export interface Tag {
  id: string;
  user_id: string;
  name: string;
  color: string;
  created_at: string;
}

export interface Interaction {
  id: string;
  entry_id: string;
  user_id: string;
  tried: boolean;
  score: number | null;
  comments: string;
  contacted_at: string | null;
  purchased_at: string | null;
  created_at: string;
  updated_at: string;
}

interface EntryListResponse {
  entries: Entry[];
  total: number;
  page: number;
  limit: number;
}

interface TagListResponse {
  tags: Tag[];
}

// Auth header helper - session is cookie-based, no token needed
function getAuthHeaders(): HeadersInit {
  // Session is handled via HTTP-only cookie with credentials: 'include'
  return {};
}

// Entries API
export async function fetchEntries(params: {
  page?: number;
  limit?: number;
  tag?: string;
  search?: string;
  status?: string;
  exclude_tried?: boolean;
} = {}): Promise<EntryListResponse> {
  const searchParams = new URLSearchParams();
  if (params.page) searchParams.set('page', params.page.toString());
  if (params.limit) searchParams.set('limit', params.limit.toString());
  if (params.tag) searchParams.set('tag', params.tag);
  if (params.search) searchParams.set('search', params.search);
  if (params.status) searchParams.set('status', params.status);
  if (params.exclude_tried) searchParams.set('exclude_tried', params.exclude_tried.toString());

  const response = await fetch(`${API_BASE}/entries?${searchParams}`, {
    headers: {
      ...getAuthHeaders(),
    },
    credentials: 'include',
  });

  if (!response.ok) {
    throw new Error(`Failed to fetch entries: ${response.statusText}`);
  }

  return response.json();
}

export async function fetchEntry(id: string): Promise<Entry> {
  const response = await fetch(`${API_BASE}/entries/${id}`, {
    headers: {
      ...getAuthHeaders(),
    },
    credentials: 'include',
  });

  if (!response.ok) {
    throw new Error(`Failed to fetch entry: ${response.statusText}`);
  }

  return response.json();
}

export async function createEntry(url: string): Promise<Entry> {
  const response = await fetch(`${API_BASE}/entries`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...getAuthHeaders(),
    },
    credentials: 'include',
    body: JSON.stringify({ url }),
  });

  if (!response.ok) {
    // Try to parse error response for friendly message
    let errorMessage = `Failed to create entry: ${response.statusText}`;
    try {
      const errorData = await response.json();
      if (errorData.detail) {
        errorMessage = errorData.detail;
      }
    } catch {
      // If parsing fails, use default message
    }
    throw new Error(errorMessage);
  }

  return response.json();
}

export async function updateEntry(id: string, data: Partial<Entry>): Promise<Entry> {
  const response = await fetch(`${API_BASE}/entries/${id}`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
      ...getAuthHeaders(),
    },
    credentials: 'include',
    body: JSON.stringify(data),
  });

  if (!response.ok) {
    throw new Error(`Failed to update entry: ${response.statusText}`);
  }

  return response.json();
}

export async function deleteEntry(id: string): Promise<void> {
  const response = await fetch(`${API_BASE}/entries/${id}`, {
    method: 'DELETE',
    headers: {
      ...getAuthHeaders(),
    },
    credentials: 'include',
  });

  if (!response.ok) {
    throw new Error(`Failed to delete entry: ${response.statusText}`);
  }
}

// Tags API
export async function fetchTags(): Promise<Tag[]> {
  const response = await fetch(`${API_BASE}/tags`, {
    headers: {
      ...getAuthHeaders(),
    },
    credentials: 'include',
  });

  if (!response.ok) {
    throw new Error(`Failed to fetch tags: ${response.statusText}`);
  }

  const data = await response.json();
  // Handle both array response and { tags: [...] } response
  return Array.isArray(data) ? data : data.tags || [];
}

export async function createTag(name: string, color: string): Promise<Tag> {
  const response = await fetch(`${API_BASE}/tags`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...getAuthHeaders(),
    },
    credentials: 'include',
    body: JSON.stringify({ name, color }),
  });

  if (!response.ok) {
    throw new Error(`Failed to create tag: ${response.statusText}`);
  }

  return response.json();
}

export async function updateTag(id: string, name: string, color: string): Promise<Tag> {
  const response = await fetch(`${API_BASE}/tags/${id}`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
      ...getAuthHeaders(),
    },
    credentials: 'include',
    body: JSON.stringify({ name, color }),
  });

  if (!response.ok) {
    throw new Error(`Failed to update tag: ${response.statusText}`);
  }

  return response.json();
}

export async function deleteTag(id: string): Promise<void> {
  const response = await fetch(`${API_BASE}/tags/${id}`, {
    method: 'DELETE',
    headers: {
      ...getAuthHeaders(),
    },
    credentials: 'include',
  });

  if (!response.ok) {
    throw new Error(`Failed to delete tag: ${response.statusText}`);
  }
}

export async function addTagToEntry(entryId: string, tagId: string): Promise<void> {
  const response = await fetch(`${API_BASE}/entries/${entryId}/tags`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...getAuthHeaders(),
    },
    credentials: 'include',
    body: JSON.stringify({ tag_id: tagId }),
  });

  if (!response.ok) {
    throw new Error(`Failed to add tag: ${response.statusText}`);
  }
}

export async function removeTagFromEntry(entryId: string, tagId: string): Promise<void> {
  const response = await fetch(`${API_BASE}/entries/${entryId}/tags/${tagId}`, {
    method: 'DELETE',
    headers: {
      ...getAuthHeaders(),
    },
    credentials: 'include',
  });

  if (!response.ok) {
    throw new Error(`Failed to remove tag: ${response.statusText}`);
  }
}

// Stores for reactive state
export const entries = writable<Entry[]>([]);
export const tags = writable<Tag[]>([]);
export const loading = writable(false);
export const error = writable<string | null>(null);

// Interaction API
export async function fetchInteraction(entryId: string): Promise<Interaction | null> {
  const response = await fetch(`${API_BASE}/entries/${entryId}/interaction`, {
    headers: {
      ...getAuthHeaders(),
    },
    credentials: 'include',
  });

  if (!response.ok) {
    throw new Error(`Failed to fetch interaction: ${response.statusText}`);
  }

  const text = await response.text();
  return text === 'null' ? null : JSON.parse(text);
}

export async function upsertInteraction(entryId: string, data: {
  tried?: boolean;
  score?: number;
  comments?: string;
  contacted_at?: string;
  purchased_at?: string;
}): Promise<Interaction> {
  const response = await fetch(`${API_BASE}/entries/${entryId}/interaction`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
      ...getAuthHeaders(),
    },
    credentials: 'include',
    body: JSON.stringify(data),
  });

  if (!response.ok) {
    throw new Error(`Failed to save interaction: ${response.statusText}`);
  }

  return response.json();
}

export async function deleteInteraction(entryId: string): Promise<void> {
  const response = await fetch(`${API_BASE}/entries/${entryId}/interaction`, {
    method: 'DELETE',
    headers: {
      ...getAuthHeaders(),
    },
    credentials: 'include',
  });

  if (!response.ok) {
    throw new Error(`Failed to delete interaction: ${response.statusText}`);
  }
}

// Random selection API
export async function fetchRandomEntry(params: {
  exclude_tried?: boolean;
  include_tags?: string[];
  exclude_tags?: string[];
} = {}): Promise<Entry> {
  const response = await fetch(`${API_BASE}/entries/random`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...getAuthHeaders(),
    },
    credentials: 'include',
    body: JSON.stringify(params),
  });

  if (!response.ok) {
    throw new Error(`Failed to get random entry: ${response.statusText}`);
  }

  return response.json();
}

export async function fetchArchiveRevisions(entryId: string): Promise<ArchiveRevision[]> {
  const response = await fetch(`${API_BASE}/entries/${entryId}/archive/revisions`, {
    headers: { ...getAuthHeaders() },
    credentials: 'include',
  });
  if (!response.ok) {
    throw new Error(`Failed to fetch archive revisions: ${response.statusText}`);
  }
  const payload = await response.json();
  return payload.revisions || [];
}

export async function refreshArchive(entryId: string): Promise<void> {
  const response = await fetch(`${API_BASE}/entries/${entryId}/archive/refresh`, {
    method: 'POST',
    headers: { ...getAuthHeaders() },
    credentials: 'include',
  });
  if (!response.ok) {
    throw new Error(`Failed to refresh archive: ${response.statusText}`);
  }
}

export async function deleteArchiveRevision(entryId: string, revisionId: string): Promise<{ deleted: boolean }> {
  const response = await fetch(`${API_BASE}/entries/${entryId}/archive/revisions/${revisionId}`, {
    method: 'DELETE',
    headers: { ...getAuthHeaders() },
    credentials: 'include',
  });
  if (!response.ok) {
    throw new Error(`Failed to delete revision: ${response.statusText}`);
  }
  return response.json();
}

export async function retryInManualMode(entryId: string, makeDefaultForDomain: boolean): Promise<void> {
  const response = await fetch(`${API_BASE}/entries/${entryId}/archive/refresh`, {
    method: 'POST',
    headers: { 
      ...getAuthHeaders(),
      'Content-Type': 'application/json',
    },
    credentials: 'include',
    body: JSON.stringify({
      manual_mode: true,
      add_to_human_domains: makeDefaultForDomain,
    }),
  });
  if (!response.ok) {
    throw new Error(`Failed to retry in manual mode: ${response.statusText}`);
  }
}

export async function addToHumanDomains(domain: string): Promise<void> {
  const response = await fetch(`${API_BASE}/admin/human-domains`, {
    method: 'POST',
    headers: { 
      ...getAuthHeaders(),
      'Content-Type': 'application/json',
    },
    credentials: 'include',
    body: JSON.stringify({ domain }),
  });
  if (!response.ok) {
    throw new Error(`Failed to add domain to human domains: ${response.statusText}`);
  }
}
