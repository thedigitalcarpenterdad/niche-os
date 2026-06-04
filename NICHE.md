# Niche OS — ClickClack Fork

This is Niche Waterproofing's customized fork of [ClickClack](https://github.com/openclaw/clickclack).

**Live at:** https://os.nichewaterproofing.com  
**Auth:** https://auth.nichewaterproofing.com (Logto)

## Niche Customizations
- Logto OIDC authentication (Phone SMS + Google OAuth)
- Niche Waterproofing branding (navy, logo)
- Topics UI — job-specific sub-channels in sidebar
- NicheBot integration
- Post-call Talkie summaries routed to job topics

## Architecture
- ClickClack (Go + SvelteKit) — team communication
- Logto — auth layer (auth.nichewaterproofing.com)
- Talkie — voice/AI layer (talkie.nichewaterproofing.com)
- All running on Hetzner CCX23 (5.161.193.159)

## Repos
- This repo: ClickClack customizations
- [niche-company-openclaw](https://github.com/thedigitalcarpenterdad/niche-company-openclaw): Talkie + infra
- [niche-claw-vault](https://github.com/thedigitalcarpenterdad/niche-claw-vault): OpenClaw workspace
