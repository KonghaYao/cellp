export const meta = {
  name: 'ingress-ad12-p2',
  description: 'AD-12 P2: migrate remaining e2e to Host, mock gateway Host, HostOnly dev, test, commit',
}

const cwd = '/Users/mino/code/remote/cellp'

phase('Migrate-e2e')
const e2eA = await agent(
  `在 ${cwd}/e2e/scripts：确保 lib.sh source lib-ingress.sh；lib-ingress.sh 含 curl_version/wait_http_200_version/http_code_version/curl_prod/wait_http_200_prod 等。
将 MANIFEST 中 ve-promote.sh ve-destroy.sh ve-fail-compensate.sh v3-dual-route.sh v5-saga-compensate.sh v6-migrate-order.sh 改为 Host 辅助（禁止新增 path GATEWAY_URL/\${PROJECT}/ 作为主路径）。
每改完 bash -n。`,
  { label: 'e2e-a', subagent_type: 'coder' },
)

phase('Migrate-e2e-b')
const e2eB = await agent(
  `在 ${cwd}/e2e/scripts 继续 Host 迁移：v1-d1-seed.sh v1-d1-branch.sh v9-kv.sh v10-queue.sh v15-archive.sh v16-worker-env.sh v17-promote-no-merge.sh v4b-promote-offshoot-fail.sh v5b-deploy-d1-branch-fail.sh。使用 lib-ingress 函数。bash -n 全部。`,
  { label: 'e2e-b', subagent_type: 'coder' },
)

phase('Mock-dev')
const mockDev = await agent(
  `1) ${cwd}/dev/mock-platform/server.mjs Gateway：当 Host 匹配 *.{base} 或 *.*.{base}（读 CELLP_INGRESS_BASE_DOMAIN 默认 ingress.local）时 proxy 到 celld，path 不 strip；保留 path 路由 deprecated 仅当 INGRESS_HOST_ONLY!=1。
2) ${cwd}/dev/.env.example 设 INGRESS_HOST_ONLY=1
3) ${cwd}/docs/plans/INGRESS-ROUTING.md 更新 P2 进度一句
go test ${cwd}/cellp/... -count=1`,
  { label: 'mock-host', subagent_type: 'coder' },
)

phase('Commit')
const commit = await agent(
  `cd ${cwd} && git add e2e/ dev/mock-platform dev/.env.example docs/plans/INGRESS-ROUTING.md cellp/ 2>/dev/null; git status; git commit -m "feat(e2e): AD-12 P2 Host-only e2e and mock gateway" 含 Co-Authored-By 若无实质改动则说明。`,
  { label: 'commit', subagent_type: 'coder' },
)

return { e2eA, e2eB, mockDev, commit }
