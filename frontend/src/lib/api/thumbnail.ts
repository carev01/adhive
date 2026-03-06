const API_BASE = '/api/v1/files';

export interface ThumbnailCandidate {
  id: string;
  entry_id: string;
  revision_id?: string;
  source_type: 'local_asset' | 'screenshot' | 'remote_meta' | 'upload';
  path: string;
  score: number;
  selected: boolean;
}

export async function fetchThumbnailCandidates(entryID: string): Promise<ThumbnailCandidate[]> {
  const response = await fetch(`${API_BASE}/thumbnails/${entryID}/candidates`, {
    credentials: 'include'
  });

  if (!response.ok) {
    throw new Error(`Failed to load thumbnail candidates: ${response.statusText}`);
  }

  const data = await response.json();
  return data.candidates || [];
}

export async function selectThumbnailCandidate(entryID: string, candidateID: string): Promise<void> {
  const response = await fetch(`${API_BASE}/thumbnails/${entryID}/select`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json'
    },
    credentials: 'include',
    body: JSON.stringify({ candidate_id: candidateID })
  });

  if (!response.ok) {
    throw new Error(`Failed to select thumbnail: ${response.statusText}`);
  }
}
