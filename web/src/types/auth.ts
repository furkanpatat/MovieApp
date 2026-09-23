export interface RegisterResponse {
  id: string;
  username: string;
  email: string;
  created_at: string;
}

export interface LoginResponse {
  /** For non-browser clients; the web app relies on the HttpOnly cookie. */
  access_token: string;
  token_type: string;
  expires_in: number;
  user: { id: string; username: string; email?: string };
}
