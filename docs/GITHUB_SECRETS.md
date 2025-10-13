# GitHub Secrets Setup Guide

This guide walks you through setting up the required GitHub secrets for automated Docker image builds.

## Required Secrets

To enable automatic Docker image building and pushing, you need these two secrets:

| Secret Name | Description | Example Value |
|-------------|-------------|---------------|
| `DOCKERHUB_USERNAME` | Your Docker Hub username | `lehuannhatrang` |
| `DOCKERHUB_TOKEN` | Docker Hub access token | `dckr_pat_xyz...` |

## Step-by-Step Setup

### 1. Create Docker Hub Access Token

1. **Log in to Docker Hub**
   - Go to https://hub.docker.com/
   - Sign in with your credentials

2. **Navigate to Security Settings**
   - Click on your username (top right)
   - Select "Account Settings"
   - Go to the "Security" tab

3. **Create New Access Token**
   - Click "New Access Token"
   - **Name**: `GitHub Actions AppBundle Operator`
   - **Permissions**: Select "Read, Write, Delete"
   - Click "Generate"

4. **Copy the Token**
   - ⚠️ **Important**: Copy the token immediately
   - You won't be able to see it again
   - It looks like: `dckr_pat_xyz123abc...`

### 2. Add Secrets to GitHub Repository

1. **Go to Repository Settings**
   - Navigate to your GitHub repository
   - Click the "Settings" tab (must be repo owner/admin)

2. **Navigate to Secrets**
   - In the left sidebar, click "Secrets and variables"
   - Click "Actions"

3. **Add DOCKERHUB_USERNAME**
   - Click "New repository secret"
   - **Name**: `DOCKERHUB_USERNAME`
   - **Secret**: `lehuannhatrang`
   - Click "Add secret"

4. **Add DOCKERHUB_TOKEN**
   - Click "New repository secret"
   - **Name**: `DOCKERHUB_TOKEN`
   - **Secret**: `dckr_pat_xyz...` (paste your token)
   - Click "Add secret"

### 3. Verify Setup

After adding the secrets, you should see:

```
Repository secrets (2)
- DOCKERHUB_USERNAME
- DOCKERHUB_TOKEN
```

## Testing the Setup

### 1. Trigger a Build

Push a commit to the main branch:

```bash
git checkout main
git add .
git commit -m "test: trigger Docker build"
git push origin main
```

### 2. Monitor the Workflow

1. Go to the "Actions" tab in your repository
2. You should see a workflow run called "Build and Push Docker Image"
3. Click on it to see the progress

### 3. Check Docker Hub

After the workflow completes successfully:

1. Go to https://hub.docker.com/r/lehuannhatrang/appbundle-operator
2. You should see new tags like:
   - `latest`
   - `main`
   - `main-<git-sha>`

## Troubleshooting

### Secret Not Working

**Symptoms:**
- Build fails with authentication error
- "Error: buildx failed with: unauthorized"

**Solutions:**
1. **Check token validity**
   - Token might have expired
   - Create a new token and update the secret

2. **Verify token permissions**
   - Ensure token has "Read, Write, Delete" permissions
   - Personal Access Tokens need sufficient scope

3. **Check repository name**
   - Ensure `lehuannhatrang/appbundle-operator` exists on Docker Hub
   - Create the repository if it doesn't exist

### Repository Not Found

**Symptoms:**
- "Error: repository does not exist or may require 'docker login'"

**Solutions:**
1. **Create Docker Hub Repository**
   - Go to https://hub.docker.com/repository/create
   - Repository name: `appbundle-operator`
   - Visibility: Public (recommended)
   - Click "Create"

2. **Check repository name in workflow**
   - Verify the image name matches: `lehuannhatrang/appbundle-operator`

### Access Denied

**Symptoms:**
- "Error: denied: requested access to the resource is denied"

**Solutions:**
1. **Check username case**
   - Docker Hub usernames are case-sensitive
   - Ensure exact match: `lehuannhatrang`

2. **Verify token ownership**
   - Token must be created by the repository owner
   - Ensure you're logged in as the correct user

## Security Best Practices

### 1. Token Permissions

- ✅ **Use minimal permissions**: Only "Read, Write, Delete" for this repository
- ❌ **Avoid admin permissions**: Not needed for pushing images

### 2. Token Rotation

- 🔄 **Rotate tokens periodically** (every 6-12 months)
- 📅 **Set calendar reminders** for token renewal
- 🔐 **Update GitHub secrets** when rotating

### 3. Repository Access

- 👥 **Limit repository access** to trusted collaborators
- 🔒 **Use branch protection** on main branch
- 📝 **Review workflow changes** carefully

### 4. Secret Management

- ❌ **Never commit secrets** to code
- ✅ **Use GitHub secrets** for sensitive data
- 🔍 **Audit secret usage** regularly

## Advanced Configuration

### Custom Registry

To use a different registry, update the workflow:

```yaml
env:
  REGISTRY: ghcr.io  # GitHub Container Registry
  IMAGE_NAME: lehuannhatrang/appbundle-operator
```

### Additional Secrets

For advanced setups, you might need:

| Secret | Purpose |
|--------|---------|
| `REGISTRY_URL` | Custom registry URL |
| `REGISTRY_USERNAME` | Registry username |
| `REGISTRY_PASSWORD` | Registry password |

### Multiple Registries

Push to multiple registries:

```yaml
- name: Login to Docker Hub
  uses: docker/login-action@v3
  with:
    username: ${{ secrets.DOCKERHUB_USERNAME }}
    password: ${{ secrets.DOCKERHUB_TOKEN }}

- name: Login to GitHub Container Registry
  uses: docker/login-action@v3
  with:
    registry: ghcr.io
    username: ${{ github.actor }}
    password: ${{ secrets.GITHUB_TOKEN }}
```

## Next Steps

After setting up the secrets:

1. ✅ **Test the workflow** by pushing to main
2. 📋 **Monitor builds** in GitHub Actions
3. 🐳 **Verify images** on Docker Hub
4. 🚀 **Update deployment** to use new images
5. 📚 **Document** your CI/CD process

## Getting Help

If you encounter issues:

1. **Check workflow logs** in GitHub Actions
2. **Review Docker Hub activity** logs
3. **Verify secret configuration** (names and values)
4. **Test authentication locally**:
   ```bash
   echo $DOCKERHUB_TOKEN | docker login -u $DOCKERHUB_USERNAME --password-stdin
   ```

## Resources

- [Docker Hub Access Tokens](https://docs.docker.com/docker-hub/access-tokens/)
- [GitHub Actions Secrets](https://docs.github.com/en/actions/security-guides/encrypted-secrets)
- [Docker Login Action](https://github.com/docker/login-action)

