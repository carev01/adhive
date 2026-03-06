const API_BASE = '/api/v1';

export interface LoginRequest {
	email: string;
	password: string;
}

export interface RegisterRequest {
	email: string;
	password: string;
	passwordConfirm: string;
}

export interface AuthResponse {
	user?: {
		id: string;
		email: string;
		display_name: string;
		created_at: string;
	};
	error?: string;
}

export async function login(credentials: LoginRequest): Promise<AuthResponse> {
	try {
		const res = await fetch(`${API_BASE}/auth/login`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(credentials),
			credentials: 'include'
		});
		const data = await res.json();
		if (!res.ok) {
			return { error: data.detail || 'Login failed' };
		}
		// Store user email for session persistence (session is in HTTP-only cookie)
		if (data.user && data.user.email) {
			localStorage.setItem('user_email', data.user.email);
			localStorage.setItem('user_id', data.user.id);
		}
		return { user: data.user };
	} catch (e) {
		return { error: 'Network error' };
	}
}

export async function register(data: RegisterRequest): Promise<AuthResponse> {
	try {
		const res = await fetch(`${API_BASE}/auth/register`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(data),
			credentials: 'include'
		});
		const response = await res.json();
		if (!res.ok) {
			return { error: response.detail || 'Registration failed' };
		}
		// Store user email for session persistence (session is in HTTP-only cookie)
		if (response.user && response.user.email) {
			localStorage.setItem('user_email', response.user.email);
			localStorage.setItem('user_id', response.user.id);
		}
		return { user: response.user };
	} catch (e) {
		return { error: 'Network error' };
	}
}

export async function logout(): Promise<void> {
	try {
		await fetch(`${API_BASE}/auth/logout`, {
			method: 'POST',
			credentials: 'include'
		});
	} finally {
		localStorage.removeItem('session_token');
		localStorage.removeItem('user_email');
		localStorage.removeItem('user_id');
	}
}
