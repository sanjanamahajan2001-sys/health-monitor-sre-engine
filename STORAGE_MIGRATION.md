# Storage Domain Migration Guide

This guide explains how to migrate from one storage domain to another without breaking existing agents.

## Quick Migration Summary

**Setting environment variables is NOT compulsory.** The agent has a default storage URL hardcoded in the code. However, to migrate existing agents to a new storage domain without breaking functionality, you have three safe options:

**Option 1 - Update Default URL in Code (Recommended for Long-term):** Update the `DefaultUpdateBaseURL` constant in `pkg/update/updater.go` to your new storage domain, rebuild the binary, and upload it to both old and new storage locations. Existing agents will continue working normally (they'll just fail to update until they get the new version, but health checks continue unaffected). New installations will automatically use the new storage. This is the cleanest long-term solution.

**Option 2 - Set Environment Variable (Quick Fix for Existing Servers):** On each server, set `UPDATE_BASE_URL` environment variable to point to the new storage domain. This overrides the default URL immediately without needing to rebuild or redeploy. Agents continue working normally and will update from the new storage. This is the fastest way to migrate existing installations.

**Option 3 - Use Redirect/Proxy (Zero Changes Needed):** Set up a redirect or reverse proxy from your old storage domain to the new one. Agents using the old URL will automatically be redirected to the new storage. No code changes, no environment variables, no server modifications needed. This provides immediate migration with zero downtime.

**Important Safety Guarantee:** Regardless of which option you choose, existing agents will NEVER break or stop functioning. The update mechanism is completely independent of the health check functionality. Even if update checks fail (due to wrong URL, network issues, etc.), the agent continues running health checks normally. The worst case scenario is that agents won't receive updates until the migration is complete, but they will continue providing health monitoring without any interruption.

## Migration Strategies

### Strategy 1: Update Default URL + Environment Variable (Recommended)

**Best for**: Gradual migration with zero downtime

#### Step 1: Update Code Default URL

Edit `pkg/update/updater.go`:
```go
const (
    DefaultUpdateBaseURL = "https://new-storage-domain.com"  // New URL
    // ...
)
```

#### Step 2: Build and Deploy New Version

```bash
VERSION=v0.4  # New version
# Build with new default URL
go build ...
```

#### Step 3: Upload to Both Storage Locations

**Old Storage (temporary):**
- Upload new binary
- Update version.txt

**New Storage:**
- Upload new binary
- Update version.txt

#### Step 4: Update Environment Variables on Servers

**Option A: Update on each server manually**
```bash
# SSH to each server
export UPDATE_BASE_URL="https://new-storage-domain.com"
echo 'export UPDATE_BASE_URL="https://new-storage-domain.com"' >> ~/.bashrc
source ~/.bashrc
```

**Option B: Use configuration management (Ansible, Puppet, etc.)**
```yaml
# Example Ansible playbook
- name: Update health-monitor config
  lineinfile:
    path: /etc/environment
    line: 'UPDATE_BASE_URL=https://new-storage-domain.com'
```

**Option C: Update via systemd service file**
```ini
[Service]
Environment="UPDATE_BASE_URL=https://new-storage-domain.com"
```

#### Step 5: Agents Auto-Update

- Agents with old default URL: Will fail to update, but continue working
- Agents with new URL (via env var): Will update successfully
- New installations: Will use new default URL

#### Step 6: Monitor and Cleanup

1. Monitor which agents have updated
2. Manually update remaining agents if needed
3. After all agents updated, remove old storage

**Timeline**: Gradual migration over days/weeks

---

### Strategy 2: Redirect/Proxy (Zero Downtime)

**Best for**: Immediate migration with no agent changes needed

#### Step 1: Set Up Redirect

**Option A: Cloudflare R2 Custom Domain Redirect**
- Keep old domain active
- Set up redirect from old to new domain
- Or use Cloudflare Workers to proxy requests

**Option B: Reverse Proxy**
- Set up nginx/HAProxy to proxy old domain to new domain
- Keep old domain active and proxy to new storage

**Option C: DNS CNAME**
- Point old domain to new domain via CNAME
- Both domains serve same content

#### Step 2: Upload Files to New Storage

- Upload binaries to new storage
- Update version.txt in new storage
- Verify new storage is accessible

#### Step 3: Configure Redirect/Proxy

- Set up redirect from old URLs to new URLs
- Test redirect works: `curl https://old-domain.com/version.txt`
- Should redirect to new domain

#### Step 4: Agents Continue Working

- Agents using old URL will be redirected to new URL
- No changes needed on agents
- Zero downtime

#### Step 5: Update Default URL in Code (Future)

- Update default URL in next release
- Old agents will gradually update and use new default
- Can remove redirect after all agents updated

**Timeline**: Immediate migration, redirect can stay indefinitely

---

### Strategy 3: Dual Storage Period

**Best for**: Maximum safety, gradual migration

#### Step 1: Set Up New Storage

- Create new storage bucket/domain
- Upload all binaries to new storage
- Update version.txt in new storage

#### Step 2: Keep Both Storages Active

- Keep old storage active
- Keep new storage active
- Both serve same files

#### Step 3: Update Environment Variables Gradually

- Update servers one by one or in batches
- Set `UPDATE_BASE_URL` to new domain
- Test each server after update

#### Step 4: Monitor

- Check which servers are using new storage
- Verify updates work from new storage
- Ensure no issues

#### Step 5: Update Default URL

- Update code default URL to new domain
- Build new version
- Upload to both storages

#### Step 6: Decommission Old Storage

- After all agents updated (or using new default)
- Remove old storage
- Update documentation

**Timeline**: 1-4 weeks depending on server count

---

## Step-by-Step: Recommended Approach

### Phase 1: Preparation (Before Migration)

1. **Set up new storage:**
   ```bash
   # Upload all existing binaries to new storage
   # Upload version.txt to new storage
   # Verify accessibility
   curl https://new-storage-domain.com/version.txt
   ```

2. **Test new storage:**
   ```bash
   # Test on one server first
   export UPDATE_BASE_URL="https://new-storage-domain.com"
   health-monitor
   # Verify update works
   ```

3. **Document current state:**
   - List all servers with health-monitor installed
   - Note which have UPDATE_BASE_URL set
   - Note current versions

### Phase 2: Update Code (Optional but Recommended)

1. **Update default URL:**
   ```go
   // pkg/update/updater.go
   const DefaultUpdateBaseURL = "https://new-storage-domain.com"
   ```

2. **Build new version:**
   ```bash
   VERSION=v0.4
   # Build with new default
   ```

3. **Upload to both storages:**
   - Upload to old storage (for backward compatibility)
   - Upload to new storage
   - Update version.txt in both

### Phase 3: Migration Execution

**Option A: Environment Variable Update (Recommended)**

1. **Update environment variables:**
   ```bash
   # For each server
   echo 'export UPDATE_BASE_URL="https://new-storage-domain.com"' >> ~/.bashrc
   source ~/.bashrc
   ```

2. **Or use systemd (if running as service):**
   ```bash
   sudo systemctl edit health-monitor
   # Add:
   [Service]
   Environment="UPDATE_BASE_URL=https://new-storage-domain.com"
   sudo systemctl daemon-reload
   ```

**Option B: Redirect Setup**

1. **Set up redirect:**
   - Configure redirect from old domain to new domain
   - Test: `curl -L https://old-domain.com/version.txt`

2. **No agent changes needed:**
   - Agents continue using old URL
   - Requests automatically redirected to new storage

### Phase 4: Verification

1. **Test updates:**
   ```bash
   # On test server
   health-monitor
   # Should update from new storage
   ```

2. **Monitor:**
   - Check error logs for update failures
   - Verify agents are updating successfully
   - Track which servers have migrated

3. **Verify functionality:**
   ```bash
   # Check version
   health-monitor --version
   
   # Check update URL
   echo $UPDATE_BASE_URL
   
   # Test update check
   curl $UPDATE_BASE_URL/version.txt
   ```

### Phase 5: Cleanup (After Migration Complete)

1. **Wait for all agents to update:**
   - Monitor for 1-2 weeks
   - Ensure all agents have new version

2. **Update default URL in code:**
   - Already done in Phase 2 if you chose that option

3. **Remove old storage (optional):**
   - Only after confirming all agents migrated
   - Or keep as backup for a period

---

## Migration Checklist

### Pre-Migration

- [ ] Set up new storage domain
- [ ] Upload all binaries to new storage
- [ ] Upload version.txt to new storage
- [ ] Verify new storage is accessible
- [ ] Test update from new storage on one server
- [ ] Document all servers with health-monitor
- [ ] Choose migration strategy

### Migration

- [ ] Update code default URL (if using Strategy 1)
- [ ] Build new version with updated default
- [ ] Upload to both storages (if using dual storage)
- [ ] Set up redirect (if using Strategy 2)
- [ ] Update environment variables (if using Strategy 1 or 3)
- [ ] Test on first server/batch
- [ ] Roll out to remaining servers

### Post-Migration

- [ ] Verify all agents updated successfully
- [ ] Monitor for errors
- [ ] Update documentation
- [ ] Remove old storage (after confirmation period)
- [ ] Update internal documentation

---

## Important Notes

### Safety Guarantees

✅ **Agents will continue working** even if update check fails
- Health checks are independent of update mechanism
- Update failures don't crash the application
- Agents function normally even if storage is unavailable

### Backward Compatibility

- Old agents (without UPDATE_BASE_URL set) will use old default URL
- They'll fail to update but continue working
- New agents (with updated default or env var) will use new URL

### Rollback Plan

If migration causes issues:

1. **Revert environment variables:**
   ```bash
   export UPDATE_BASE_URL="https://old-storage-domain.com"
   ```

2. **Or revert code default URL:**
   - Change back to old URL
   - Rebuild and deploy

3. **Keep old storage active** until migration is confirmed successful

---

## Example: Migrating from R2 to AWS S3

### Scenario
- **Old**: `https://pub-0cf86c67f6dd456087607707a5a37f73.r2.dev`
- **New**: `https://health-monitor-releases.s3.amazonaws.com`

### Steps

1. **Upload files to S3:**
   ```bash
   aws s3 cp version.txt s3://health-monitor-releases/version.txt --acl public-read
   aws s3 cp health-monitor-linux-amd64-v0.3 s3://health-monitor-releases/ --acl public-read
   ```

2. **Update code:**
   ```go
   const DefaultUpdateBaseURL = "https://health-monitor-releases.s3.amazonaws.com"
   ```

3. **Update environment variables on servers:**
   ```bash
   export UPDATE_BASE_URL="https://health-monitor-releases.s3.amazonaws.com"
   ```

4. **Or set up CloudFront/redirect:**
   - Point old R2 domain to S3
   - Or use CloudFront to serve from S3

---

## Troubleshooting Migration

### Issue: Agents not updating after migration

**Check:**
1. Verify new storage is accessible
2. Check UPDATE_BASE_URL is set correctly
3. Test URL manually: `curl $UPDATE_BASE_URL/version.txt`
4. Check for network/firewall issues

### Issue: Some agents still using old storage

**Solution:**
- Update environment variables on those servers
- Or wait for them to update to new version (which has new default)

### Issue: Update failures during migration

**Solution:**
- Agents continue working despite update failures
- Fix storage issues
- Agents will retry on next run

---

## Best Practices

1. **Test on one server first** before rolling out
2. **Keep old storage active** during migration period
3. **Monitor update success rates** after migration
4. **Document the process** for future migrations
5. **Have a rollback plan** ready
6. **Update default URL in code** for new installations
7. **Use environment variables** for flexibility

---

## Summary

**Zero-Downtime Migration Options:**

1. **Environment Variable Update** - Update UPDATE_BASE_URL on each server
2. **Redirect/Proxy** - Set up redirect from old to new domain
3. **Dual Storage** - Keep both active, migrate gradually

**Key Points:**
- ✅ Agents continue working even if update fails
- ✅ No breaking changes to functionality
- ✅ Can migrate gradually
- ✅ Easy rollback if needed
- ✅ Zero downtime possible

Choose the strategy that best fits your infrastructure and operational preferences.
