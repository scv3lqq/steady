# Private account documentation edits: .env.example

Base: `34a5c2d`. Apply and check with the canonical prose-edit helper; change only this file.

**E1.** (lines 9-9)

````md E1-old
STEADY_ALLOWED_HOSTS=
````

````md E1-new
STEADY_ALLOWED_HOSTS=
# Keep true for an HTTPS reverse proxy. Forwarded headers never choose cookie security.
# Set false only for local loopback HTTP or an SSH tunnel, then restart steady.
STEADY_COOKIE_SECURE=true
````
