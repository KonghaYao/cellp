export const meta = {
  name: 'ingress-ad12-p1',
  description: 'AD-12 P1: e2e Host helpers, v4 cutover, site preview doc, go test, commit',
}

const cwd = '/Users/mino/code/remote/cellp'
const spec = `${cwd}/docs/plans/INGRESS-ROUTING.md`

phase('Implement-parallel')
const [e2eBatch, siteDoc] = await parallel([
  () =>
    agent(
      `在 ${cwd} 完成 AD-12 P1 e2e：
1) 若 ${cwd}/e2e/scripts/lib.sh 尚无 Host 辅助函数，补充：ingress_base_domain、curl_gateway_host、http_code_gateway_host、wait_http_200_host、version_preview_url（读 API preview_url）
2) 更新 v4-promote-cutover.sh：prod 用 prod Host（${DEV_PROJECT}.ingress.local 风格）；preview 用 API preview_url 或 Host；path URL 作 fallback
3) 扫描 e2e/scripts/*.sh 中 GATEWAY_URL/\${PROJECT}/ 用法，至少再改 v3-dual-route.sh 或 ve-promote.sh 之一支持 Host（与 lib 一致）
4) dev/.env.example 增加 CELLP_INGRESS_BASE_DOMAIN=ingress.local 注释（/etc/hosts）
勿改 support-corpus。写代码后 cd cellp && go test ./... 必须通过（若只改 shell 则 bash -n 脚本）。`,
      { label: 'e2e-host', subagent_type: 'coder' },
    ),
  () =>
    agent(
      `更新 ${cwd}/site/docs/concepts/preview.md 与 ${cwd}/dev/README.md（简短）：AD-12 Host 路由、preview Host 形如 {version}.{project}.ingress.local、dev 需 /etc/hosts 127.0.0.1 行；path 路由 deprecated。对齐 ${spec}。`,
      { label: 'site-dev-doc', subagent_type: 'coder' },
    ),
])

phase('Verify')
const verify = await agent(
  `在 ${cwd}：cd cellp && go test ./... -count=1；grep -l curl_gateway_host e2e/scripts/lib.sh e2e/scripts/v4-promote-cutover.sh。列出改动文件。`,
  { label: 'verify', subagent_type: 'verification' },
)

phase('Commit')
const commit = await agent(
  `在 ${cwd} git add e2e/ site/docs/concepts/preview.md dev/README.md dev/.env.example docs/plans/INGRESS-ROUTING.md（若有改）cellp/internal/orch/（若有改）；不要 add support-corpus workflow-runs coverage.out。
git commit -m "feat(e2e): AD-12 P1 Host routing for promote cutover" 正文说明 e2e Host + prod binding；末尾 Co-Authored-By: composer-2.5-fast <noreply@anthropic.com>
若无改动则说明原因不提交。`,
  { label: 'commit', subagent_type: 'coder' },
)

return { e2eBatch, siteDoc, verify, commit }
