-- Development only. Run explicitly after migrations, never at application startup.
INSERT INTO shops (id, name, slug, timezone)
VALUES ('11111111-1111-4111-8111-111111111111', 'Barbearia do Felipe', 'barbearia-do-felipe', 'America/Sao_Paulo')
ON CONFLICT (slug) DO NOTHING;
