# CI/CD Setup Guide

This guide explains how to set up continuous integration and continuous deployment for the AppBundle Operator.

## GitHub Actions Workflows

The project includes automated workflows for:
- **Testing**: Run tests on every push and pull request
- **Docker Build & Push**: Build and push Docker images on commits to main

## Docker Hub Integration

### Prerequisites

1. **Docker Hub Account**: You need a Docker Hub account
2. **Repository**: The repository `lehuannhatrang/appbundle-operator` should exist on Docker Hub

### Setting up Docker Hub Secrets

To enable automatic image pushing, you need to configure GitHub secrets:

1. **Go to GitHub Repository Settings**
   - Navigate to your GitHub repository
   - Click on "Settings" tab
   - Click on "Secrets and variables" → "Actions"

2. **Add Required Secrets**
   
   Click "New repository secret" and add these two secrets:

   **DOCKERHUB_USERNAME**
   - Name: `DOCKERHUB_USERNAME`
   - Value: `lehuannhatrang` (your Docker Hub username)

   **DOCKERHUB_TOKEN**
   - Name: `DOCKERHUB_TOKEN`
   - Value: Your Docker Hub access token (see below how to create)

### Creating Docker Hub Access Token

1. **Log in to Docker Hub**
   - Go to https://hub.docker.com/
   - Log in with your credentials

2. **Create Access Token**
   - Click on your username → "Account Settings"
   - Go to "Security" tab
   - Click "New Access Token"
   - Give it a name like "GitHub Actions AppBundle Operator"
   - Select permissions: "Read, Write, Delete"
   - Click "Generate"
   - **Copy the token immediately** (you won't see it again)

3. **Add Token to GitHub Secrets**
   - Use this token as the value for `DOCKERHUB_TOKEN` secret

## Workflow Details

### Main Workflow: `.github/workflows/docker-build-push.yml`

This workflow:

**Triggers:**
- Push to `main` branch
- New tags (e.g., `v1.0.0`)
- Pull requests (testing only, no push)

**Jobs:**

1. **Test Job** (runs on all triggers)
   - Sets up Go environment
   - Caches dependencies
   - Runs `make test`
   - Runs `make vet`
   - Checks code formatting

2. **Build and Push Job** (only on main branch and tags)
   - Builds multi-platform Docker image (amd64, arm64)
   - Pushes to Docker Hub with multiple tags
   - Uses build cache for faster builds

**Generated Tags:**
- `latest` (for main branch)
- `main` (for main branch)
- `v1.0.0` (for version tags)
- `1.0` (for version tags)
- `1` (for version tags)
- `main-<git-sha>` (for main branch commits)

## Usage Examples

### Automatic Build on Main Branch

```bash
# Any push to main will trigger a build
git checkout main
git add .
git commit -m "feat: add new feature"
git push origin main

# This will create:
# - lehuannhatrang/appbundle-operator:latest
# - lehuannhatrang/appbundle-operator:main
# - lehuannhatrang/appbundle-operator:main-<sha>
```

### Release with Version Tags

```bash
# Create and push a version tag
git tag v1.0.0
git push origin v1.0.0

# This will create:
# - lehuannhatrang/appbundle-operator:v1.0.0
# - lehuannhatrang/appbundle-operator:1.0
# - lehuannhatrang/appbundle-operator:1
# - lehuannhatrang/appbundle-operator:latest (if on main)
```

### Using the Built Images

```bash
# Pull the latest version
docker pull lehuannhatrang/appbundle-operator:latest

# Pull a specific version
docker pull lehuannhatrang/appbundle-operator:v1.0.0

# Deploy to Kubernetes
make deploy IMG=lehuannhatrang/appbundle-operator:latest
```

## Local Testing

Test the workflow components locally:

### Run Tests

```bash
make test
make vet
make fmt
```

### Build Docker Image

```bash
# Build locally (same as CI)
docker buildx build --platform linux/amd64,linux/arm64 -t lehuannhatrang/appbundle-operator:test .

# Build for single platform
docker build -t lehuannhatrang/appbundle-operator:test .
```

### Test Multi-Platform Build

```bash
# Set up buildx (if not already done)
docker buildx create --use

# Build multi-platform
docker buildx build --platform linux/amd64,linux/arm64 -t lehuannhatrang/appbundle-operator:test .
```

## Monitoring CI/CD

### View Workflow Status

1. **GitHub Actions Tab**
   - Go to your repository
   - Click "Actions" tab
   - View running/completed workflows

2. **Workflow Badges** (optional)
   
   Add to your README.md:
   ```markdown
   ![Docker Build](https://github.com/<username>/<repo>/actions/workflows/docker-build-push.yml/badge.svg)
   ```

### Docker Hub Monitoring

1. **Check Repository**
   - Go to https://hub.docker.com/r/lehuannhatrang/appbundle-operator
   - View pushed tags and build dates

2. **Pull Statistics**
   - Monitor download counts
   - View tag popularity

## Troubleshooting

### Common Issues

**1. Authentication Failed**
```
Error: buildx failed with: error: failed to solve: failed to push
```
- **Solution**: Check DOCKERHUB_USERNAME and DOCKERHUB_TOKEN secrets
- Verify Docker Hub access token is valid
- Ensure repository exists on Docker Hub

**2. Tests Failing**
```
Process completed with exit code 1
```
- **Solution**: Run tests locally first
- Fix any failing tests before pushing
- Check `make test` and `make vet` output

**3. Build Platform Issues**
```
Error: multiple platforms feature is currently not supported
```
- **Solution**: The workflow uses buildx for multi-platform builds
- This shouldn't happen in GitHub Actions environment
- If testing locally, ensure buildx is set up

**4. Repository Not Found**
```
Error: failed to push: repository does not exist
```
- **Solution**: Create the repository on Docker Hub first
- Go to https://hub.docker.com/repository/create
- Create public repository named `appbundle-operator`

### Debug Steps

1. **Check Secrets**
   ```bash
   # In GitHub Actions, you can't see secret values
   # But you can check if they're set by echoing lengths:
   echo "Username length: ${#DOCKERHUB_USERNAME}"
   echo "Token length: ${#DOCKERHUB_TOKEN}"
   ```

2. **Test Docker Login Locally**
   ```bash
   # Test your credentials locally
   echo $DOCKERHUB_TOKEN | docker login -u $DOCKERHUB_USERNAME --password-stdin
   ```

3. **Validate Dockerfile**
   ```bash
   # Build locally to test Dockerfile
   docker build -t test .
   ```

## Advanced Configuration

### Custom Workflow Triggers

Edit `.github/workflows/docker-build-push.yml`:

```yaml
on:
  push:
    branches: [main, develop]  # Add more branches
    tags: ['v*']
  schedule:
    - cron: '0 2 * * 1'  # Weekly builds on Monday 2 AM
```

### Additional Tags

Add custom tagging logic:

```yaml
tags: |
  type=ref,event=branch
  type=ref,event=pr
  type=semver,pattern={{version}}
  type=raw,value=nightly,enable={{is_default_branch}}
  type=raw,value={{date 'YYYYMMDD'}},enable={{is_default_branch}}
```

### Build Arguments

Pass build arguments:

```yaml
- name: Build and push Docker image
  uses: docker/build-push-action@v5
  with:
    build-args: |
      VERSION=${{ github.ref_name }}
      BUILD_DATE=${{ github.event.head_commit.timestamp }}
```

### Notifications

Add Slack/Discord notifications:

```yaml
- name: Notify on success
  if: success()
  uses: 8398a7/action-slack@v3
  with:
    status: success
    webhook_url: ${{ secrets.SLACK_WEBHOOK }}
```

## Security Considerations

1. **Access Token Scope**: Use minimal required permissions
2. **Secret Rotation**: Rotate Docker Hub tokens periodically
3. **Branch Protection**: Protect main branch to prevent unauthorized pushes
4. **Dependency Scanning**: Consider adding vulnerability scanning

```yaml
- name: Run Trivy vulnerability scanner
  uses: aquasecurity/trivy-action@master
  with:
    image-ref: '${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:latest'
```

## Next Steps

1. **Set up the secrets** as described above
2. **Push a commit to main** to trigger the first build
3. **Check Docker Hub** for the pushed image
4. **Create a release tag** to test version tagging
5. **Update deployment scripts** to use the new image repository

## Resources

- [GitHub Actions Documentation](https://docs.github.com/en/actions)
- [Docker Hub Documentation](https://docs.docker.com/docker-hub/)
- [Docker Buildx Documentation](https://docs.docker.com/buildx/)
- [GitHub Actions for Docker](https://docs.docker.com/ci-cd/github-actions/)

