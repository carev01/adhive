import { writable, get } from 'svelte/store';
import { goto } from '$app/navigation';
import { page } from '$app/stores';
import { getCSRFToken, clearCSRFToken } from './csrf';

// Base API URL - would typically come from env
export const API_BASE = '/api/v1';

// Helper to handle API responses and auto-redirect on 401
async function handleApiResponse(response: Response): Promise<Response> {
  if (response.status === 401) {
    // Clear any stored auth state
    localStorage.removeItem('auth_token');
    // Clear CSRF token on auth failure
    clearCSRFToken();
    // Get current URL for return redirect
    const currentUrl = get(page).url.pathname;
    const returnUrl = encodeURIComponent(currentUrl);
    // Redirect to login page with return URL
    await goto(`/login?returnUrl=${returnUrl}`);
    throw new Error('Unauthorized - redirecting to login');
  }
  return response;
}

// Helper to make API calls with automatic 401 handling and CSRF token
async function apiFetch(url: string, options: RequestInit = {}): Promise<Response> {
  const method = (options.method || 'GET').toUpperCase();
  const isMutation = ['POST', 'PUT', 'DELETE', 'PATCH'].includes(method);
  
  // Get CSRF token for mutations
  const csrfToken = isMutation ? await getCSRFToken() : '';
  
  // Build headers
  const headers = new Headers(options.headers as Record<string, string>);
  headers.set('X-CSRF-Token', csrfToken);
  
  // Add CSRF token header for mutations
  const response = await fetch(url, {
    ...options,
    headers,
    credentials: 'include',
  });
  
  await handleApiResponse(response);
  return response;
}

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
  date_from?: string;
  date_to?: string;
  source?: string;
  location?: string;
  sort_by?: 'created_at' | 'updated_at' | 'title';
  sort_order?: 'asc' | 'desc';
  has_interaction?: boolean;
  min_score?: number;
} = {}): Promise<EntryListResponse> {
  const searchParams = new URLSearchParams();
  if (params.page) searchParams.set('page', params.page.toString());
  if (params.limit) searchParams.set('limit', params.limit.toString());
  if (params.tag) searchParams.set('tag', params.tag);
  if (params.search) searchParams.set('search', params.search);
  if (params.status) searchParams.set('status', params.status);
  if (params.exclude_tried) searchParams.set('exclude_tried', params.exclude_tried.toString());
  if (params.date_from) searchParams.set('date_from', params.date_from);
  if (params.date_to) searchParams.set('date_to', params.date_to);
  if (params.source) searchParams.set('source', params.source);
  if (params.location) searchParams.set('location', params.location);
  if (params.sort_by) searchParams.set('sort_by', params.sort_by);
  if (params.sort_order) searchParams.set('sort_order', params.sort_order);
  if (params.has_interaction) searchParams.set('has_interaction', params.has_interaction.toString());
  if (params.min_score) searchParams.set('min_score', params.min_score.toString());

  const response = await apiFetch(`${API_BASE}/entries?${searchParams}`);

  if (!response.ok) {
    throw new Error(`Failed to fetch entries: ${response.statusText}`);
  }

  return response.json();
}

// Sources API - fetch unique sources/domains from user's entries
export async function fetchSources(): Promise<string[]> {
  const response = await apiFetch(`${API_BASE}/entries/sources`);

  if (!response.ok) {
    throw new Error(`Failed to fetch sources: ${response.statusText}`);
  }

  const data = await response.json();
  return data.sources || [];
}

export async function fetchLocations(): Promise<string[]> {
  const response = await apiFetch(`${API_BASE}/entries/locations`);

  if (!response.ok) {
    throw new Error(`Failed to fetch locations: ${response.statusText}`);
  }

  const data = await response.json();
  return data.locations || [];
}

// Bulk operations
export async function bulkTagEntries(entryIds: string[], tagIds: string[], action: 'add' | 'remove'): Promise<void> {
  const response = await apiFetch(`${API_BASE}/entries/bulk/tag`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ entry_ids: entryIds, tag_ids: tagIds, action }),
  });

  if (!response.ok) {
    throw new Error(`Failed to ${action} tags: ${response.statusText}`);
  }
}

export async function bulkDeleteEntries(entryIds: string[]): Promise<void> {
  const response = await apiFetch(`${API_BASE}/entries/bulk/delete`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ entry_ids: entryIds }),
  });

  if (!response.ok) {
    throw new Error(`Failed to delete entries: ${response.statusText}`);
  }
}

export async function bulkArchiveEntries(entryIds: string[]): Promise<void> {
  const response = await apiFetch(`${API_BASE}/entries/bulk/archive`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ entry_ids: entryIds }),
  });

  if (!response.ok) {
    throw new Error(`Failed to archive entries: ${response.statusText}`);
  }
}

export async function fetchEntry(id: string): Promise<Entry> {
  const response = await apiFetch(`${API_BASE}/entries/${id}`);

  if (!response.ok) {
    throw new Error(`Failed to fetch entry: ${response.statusText}`);
  }

  return response.json();
}

export async function createEntry(url: string): Promise<Entry> {
  const response = await apiFetch(`${API_BASE}/entries`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ url }),
  });

  if (!response.ok) {
    throw new Error(`Failed to create entry: ${response.statusText}`);
  }

  return response.json();
}

export async function updateEntry(id: string, data: Partial<Entry>): Promise<Entry> {
  const response = await apiFetch(`${API_BASE}/entries/${id}`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(data),
  });

  if (!response.ok) {
    throw new Error(`Failed to update entry: ${response.statusText}`);
  }

  return response.json();
}

export async function deleteEntry(id: string): Promise<void> {
  const response = await apiFetch(`${API_BASE}/entries/${id}`, {
    method: 'DELETE',
  });

  if (!response.ok) {
    throw new Error(`Failed to delete entry: ${response.statusText}`);
  }
}

// Tags API
export async function fetchTags(): Promise<Tag[]> {
  const response = await apiFetch(`${API_BASE}/tags`);

  if (!response.ok) {
    throw new Error(`Failed to fetch tags: ${response.statusText}`);
  }

  const data = await response.json();
  // Handle both array response and { tags: [...] } response
  return Array.isArray(data) ? data : data.tags || [];
}

export async function createTag(name: string, color: string): Promise<Tag> {
  const response = await apiFetch(`${API_BASE}/tags`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ name, color }),
  });

  if (!response.ok) {
    throw new Error(`Failed to create tag: ${response.statusText}`);
  }

  return response.json();
}

export async function updateTag(id: string, name: string, color: string): Promise<Tag> {
  const response = await apiFetch(`${API_BASE}/tags/${id}`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ name, color }),
  });

  if (!response.ok) {
    throw new Error(`Failed to update tag: ${response.statusText}`);
  }

  return response.json();
}

export async function deleteTag(id: string): Promise<void> {
  const response = await apiFetch(`${API_BASE}/tags/${id}`, {
    method: 'DELETE',
  });

  if (!response.ok) {
    throw new Error(`Failed to delete tag: ${response.statusText}`);
  }
}

export async function addTagToEntry(entryId: string, tagId: string): Promise<void> {
  const response = await apiFetch(`${API_BASE}/entries/${entryId}/tags`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ tag_id: tagId }),
  });

  if (!response.ok) {
    throw new Error(`Failed to add tag: ${response.statusText}`);
  }
}

export async function removeTagFromEntry(entryId: string, tagId: string): Promise<void> {
  const response = await apiFetch(`${API_BASE}/entries/${entryId}/tags/${tagId}`, {
    method: 'DELETE',
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
  const response = await apiFetch(`${API_BASE}/entries/${entryId}/interaction`);

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
  const response = await apiFetch(`${API_BASE}/entries/${entryId}/interaction`, {
    method: 'PUT',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(data),
  });

  if (!response.ok) {
    throw new Error(`Failed to save interaction: ${response.statusText}`);
  }

  return response.json();
}

export async function deleteInteraction(entryId: string): Promise<void> {
  const response = await apiFetch(`${API_BASE}/entries/${entryId}/interaction`, {
    method: 'DELETE',
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
  const response = await apiFetch(`${API_BASE}/entries/random`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(params),
  });

  if (!response.ok) {
    throw new Error(`Failed to get random entry: ${response.statusText}`);
  }

  return response.json();
}

export async function fetchArchiveRevisions(entryId: string): Promise<ArchiveRevision[]> {
  const response = await apiFetch(`${API_BASE}/entries/${entryId}/archive/revisions`);
  if (!response.ok) {
    throw new Error(`Failed to fetch archive revisions: ${response.statusText}`);
  }
  const payload = await response.json();
  return payload.revisions || [];
}

export async function refreshArchive(entryId: string): Promise<void> {
  const response = await apiFetch(`${API_BASE}/entries/${entryId}/archive/refresh`, {
    method: 'POST',
  });
  if (!response.ok) {
    throw new Error(`Failed to refresh archive: ${response.statusText}`);
  }
}

export async function deleteArchiveRevision(entryId: string, revisionId: string): Promise<{ deleted: boolean }> {
  const response = await apiFetch(`${API_BASE}/entries/${entryId}/archive/revisions/${revisionId}`, {
    method: 'DELETE',
  });
  if (!response.ok) {
    throw new Error(`Failed to delete revision: ${response.statusText}`);
  }
  return response.json();
}

export async function retryInManualMode(entryId: string, makeDefaultForDomain: boolean): Promise<void> {
  const response = await apiFetch(`${API_BASE}/entries/${entryId}/archive/refresh`, {
    method: 'POST',
    headers: { 
      'Content-Type': 'application/json',
    },
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
  const response = await apiFetch(`${API_BASE}/admin/human-domains`, {
    method: 'POST',
    headers: { 
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ domain }),
  });
  if (!response.ok) {
    throw new Error(`Failed to add domain to human domains: ${response.statusText}`);
  }
}
