# Quick Docker Setup

## 🚀 One-Time Setup (5 minutes)

### 1️⃣ Get Docker Hub Token
1. Go to https://hub.docker.com/settings/security
2. Click "New Access Token"
3. Name: `GitHub Actions AppBundle`
4. Permissions: **Read, Write, Delete**
5. **Copy the token** (starts with `dckr_pat_`)

### 2️⃣ Add GitHub Secrets
1. Go to your repo → Settings → Secrets and variables → Actions
2. Add these two secrets:

| Name | Value |
|------|-------|
| `DOCKERHUB_USERNAME` | `lehuannhatrang` |
| `DOCKERHUB_TOKEN` | `dckr_pat_xyz...` (your token) |

### 3️⃣ Create Docker Hub Repository
1. Go to https://hub.docker.com/repository/create
2. Name: `appbundle-operator`
3. Visibility: **Public**
4. Click **Create**

## ✅ Test It Works

Push any commit to main branch:
```bash
git add .
git commit -m "test: trigger docker build"
git push origin main
```

Check:
- 🔍 GitHub Actions tab for build status  
- 🐳 https://hub.docker.com/r/lehuannhatrang/appbundle-operator for new image

## 📦 Use Your Image

```bash
# Pull latest
docker pull lehuannhatrang/appbundle-operator:latest

# Deploy to Kubernetes  
make deploy IMG=lehuannhatrang/appbundle-operator:latest
```

## 🏷️ Tags Created

Every commit to main creates:
- `latest`
- `main`  
- `main-<git-sha>`

Every version tag (e.g., `v1.0.0`) creates:
- `v1.0.0`
- `1.0`
- `1`

---

**Need help?** See [detailed guide](docs/GITHUB_SECRETS.md) or [CI/CD docs](docs/CICD.md)

