This is a quick hack to use contacts from one's Google account with mutt.

Create a project that has access to the People API, and put the secrets json at
`~/.contacts-secrets.json`.

The tool will attempt to open a browser window if needed. The command then works
like a normal mutt `query_command` script.
