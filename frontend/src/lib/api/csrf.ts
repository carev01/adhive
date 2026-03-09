import { API_BASE } from './client';

// CSRF token cache
let csrfToken: string | null = null;

/**
 * Fetch CSRF token from the backend
 * @returns The CSRF token string
 */
export async function getCSRFToken(): Promise<string> {
  // Return cached token if available
  if (csrfToken) {
    return csrfToken;
  }

  try {
    const response = await fetch(`${API_BASE}/csrf-token`, {
      method: 'GET',
      credentials: 'include',
    });

    if (!response.ok) {
      console.warn('Failed to fetch CSRF token:', response.statusText);
      return '';
    }

    const data = await response.json();
    const token = data.token || '';
    csrfToken = token;
    return token;
  } catch (error) {
    console.warn('Error fetching CSRF token:', error);
    return '';
  }
}

/**
 * Fetch wrapper that automatically adds CSRF token to mutations
 * @param url The URL to fetch
 * @param options Fetch options
 * @returns Response from the fetch call
 */
export async function fetchWithCSRF(url: string, options: RequestInit = {}): Promise<Response> {
  const token = await getCSRFToken();
  
  const headers = new Headers(options.headers);
  
  // Only add CSRF token for mutation methods
  if (token && ['POST', 'PUT', 'DELETE', 'PATCH'].includes((options.method || 'GET').toUpperCase())) {
    headers.set('X-CSRF-Token', token);
  }
  
  return fetch(url, {
    ...options,
    headers,
    credentials: 'include',
  });
}

/**
 * Clear the cached CSRF token (useful on logout)
 */
export function clearCSRFToken(): void {
  csrfToken = null;
}

/**
 * Force refresh the CSRF token
 * @returns The new CSRF token
 */
export async function refreshCSRFToken(): Promise<string> {
  csrfToken = null;
  return getCSRFToken();
}
