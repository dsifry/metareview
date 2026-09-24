# Keeper MVP epic — agent/subagent telemetry snapshot (2026-09-22)

Frozen copy of the agent-run telemetry produced while dogfooding metareview on the
Keeper MVP epic (WU01–WU17, ~12 h wall clock overnight 2026-09-21/22). Captured so a
future analysis agent (e.g. for the metareview 0.13.0 candidates in
`docs/0.13.0-candidates.md`) can review how the work was actually orchestrated without
relying on volatile temp directories.

## Contents of `keeper-telemetry.tar.gz` (214 files, ~1.0 MB compressed)

```
subagent-artifacts/     # per-run: <runId>_<agent>_<i>_{input.md,output.md,transcript.jsonl,meta.json}
async-subagent-runs/    # workflow runs: mission.json, status.json, events.jsonl, control/, output logs
sweep-logs/             # /tmp/wu16-bg*.log (mutation sweep milestones), /tmp/v*.out (per-unit verify)
```

Extract with:

```bash
tar -xzf keeper-telemetry.tar.gz -C /tmp/keeper-telemetry
```

## Per-run telemetry schema (`subagent-artifacts/*_meta.json`)

`runId`, `agent`, `task`, `exitCode`, `model`, `attemptedModels`, `modelAttempts`,
`usage.{input,output,cacheRead,cacheWrite,cost,turns}`, `durationMs`, `toolCount`,
`launchContractDigest`, `launchResolvedExtensions`, `acceptance`
(`status`, `evidenceStatus`, `explicit`, `effectiveAcceptance.{level,criteria,evidence}`,
`childReport` with `criteriaSatisfied`, `commandsRun`, `validationOutput`, `reviewFindings`,
`residualRisks`), `effects`, `transcriptPath`, `timestamp`.

`transcript.jsonl` holds every tool call and result. `output.md` is the final report.

## Run index (12 retained runs)

| runId | agent | model | turns | input tok | output tok | ms | exit | artifact |
|---|---|---|---:|---:|---:|---:|---:|---|
| e5cb78e8 | worker | `lunaroute/deepseek-4.1-flash-background:high` | 24 | 3028028 | 15919 | 151950 | 0 | `e5cb78e8_worker_0_meta.json` |
| 141c03f8 | worker | `lunaroute/deepseek-4.1-flash-background:high` | 5 | 675041 | 6028 | 2299272 | 0 | `141c03f8_worker_0_meta.json` |
| 2efc753a | worker | `lunaroute/deepseek-4.1-flash-background:high` | 58 | 12039344 | 33798 | 539011 | 0 | `2efc753a_worker_0_meta.json` |
| bc1658b5 | worker | `lunaroute/deepseek-4.1-flash-background:high` | 31 | 1932196 | 22902 | 2451484 | 0 | `bc1658b5_worker_0_meta.json` |
| 38e3275b | delegate | `lunaroute/deepseek-4.1-flash-background` | 1 | 0 | 0 | 2463 | 1 | `38e3275b_delegate_0_meta.json` |
| 9817748c | worker | `lunaroute/deepseek-4.1-flash-background:high` | 1 | 36170 | 444 | 1800008 | 1 | `9817748c_worker_0_meta.json` |
| 5355e302 | worker | `lunaroute/deepseek-4.1-flash-background:high` | 15 | 143237 | 9734 | 2765488 | 0 | `5355e302_worker_0_meta.json` |
| 08ac5eda | worker | `lunaroute/deepseek-4.1-flash-background:high` | 23 | 2548355 | 6828 | 10799832 | 143 | `08ac5eda_worker_0_meta.json` |
| 674f8079 | worker | `lunaroute/deepseek-4.1-flash-background:high` | 2 | 143062 | 757 | 12599895 | 143 | `674f8079_worker_0_meta.json` |
| b4914e72 | delegate | `lunaroute/deepseek-4.1-flash-background` | 1 | 0 | 0 | 2377 | 1 | `b4914e72_delegate_0_meta.json` |
| bc39ce1f | worker | `lunaroute/deepseek-4.1-flash-background:high` | 4 | 148543 | 566 | 1800008 | 1 | `bc39ce1f_worker_0_meta.json` |
| 7037b41b | worker | `lunaroute/deepseek-4.1-flash-background:high` | 10 | 130435 | 2319 | 10799798 | 143 | `7037b41b_worker_0_meta.json` |

## Live locations (durable vs volatile)

| Location | Durable? | Notes |
|---|---|---|
| `<repo>/.pi/subagents/artifacts/` | **yes** (repo dir) | per-run input/output/transcript/meta; not gitignored — no retention policy |
| `$TMPDIR/pi-subagents-<uid>/async-subagent-runs/` | **NO — volatile** | `$TMPDIR` is purged periodically (it deleted worktrees at 00:00 during this epic); only 20 of 106 dirs retained full `events.jsonl`/`status.json` |
| `~/.pi/agent/sessions/*.jsonl` | yes (home) | parent session transcripts |
| `/tmp/wu16-bg*.log`, `/tmp/v*.out` | **NO — volatile** | the mutation-sweep and verify evidence; frozen into this snapshot |

## Gaps an analysis agent should know (see `0.13.0-candidates.md` §8)

1. No cross-run index or normalized schema — you must glob and join on `runId`.
2. Workflow/async telemetry is written under `$TMPDIR` and is lost on purge; there is no
   durable default and no retention policy.
3. `.pi/subagents/artifacts` is untracked but not gitignored, so it can be committed by
   accident.
4. Nothing correlates a metareview review run (`docs/metareview/reviews/*.md`) with the
   subagent runs that produced the reviewable work, so cost/outcome attribution requires
   manual matching.

## Async-run directories captured

- `.active-runs`
- `020c0259`
- `0379cf0a-2c7f-4108-968a-cf0856acabb0`
- `0627953d-7a32-4a43-abd6-67124b9f125b`
- `068b577c-1b08-4c07-bab0-e5daa05741f2`
- `08597b48`
- `09c40263`
- `0b9ac395`
- `0d606808`
- `0ff47d43-b4a5-482c-b53b-8c960ec7a227`
- `10d4d30d`
- `13bd45ed-442c-4c4a-9c74-002ee44bfd3d`
- `1b398b14-81c8-4c28-b68d-b2bd70357664`
- `1b665d6e`
- `21287cfe-9da6-4aad-a8a6-53303316f009`
- `215a3d8c`
- `21c20563`
- `23d50d12-f179-44fa-af9b-9f6487fde770`
- `257ac3bc`
- `26b9e235`
- `291b82b4`
- `2d781f94-9494-4a93-b531-c226c44751a1`
- `328f3b0c`
- `32d76f07`
- `3473a332`
- `36d18afe`
- `3745ef88`
- `398ea545-fe2c-4eac-88a3-954f21a95c18`
- `3f32c82f-cb65-4ce0-b62e-5ae00e99dad2`
- `41b88ca1`
- `41bb9d0a-215a-4791-81b8-ddd203b7d0dc`
- `42a6da18-37a5-441d-bf91-3ec7091d3a78`
- `4fc0d7fc-fec0-4249-9c56-66c9ab59085d`
- `522315dd`
- `552799c9`
- `56e44fbe`
- `59e6b6fe`
- `5db2890b-0181-4615-9d9e-b6ea7bc9ec52`
- `5f36e7ec`
- `6067bc71-6326-4cce-87c0-96624a348849`
- `630690d1`
- `64561679`
- `672ec18a`
- `67db5f6c-ff3f-46e3-a2bc-6703c125f419`
- `680930a0-7323-4ef4-b503-029795ecc8b8`
- `6e300196-2d2f-494b-ba8f-02e148ab33d5`
- `70300a96-cb40-41b6-a32b-a165665000ac`
- `708725db`
- `71df1025`
- `741debb1`
- `760205f3`
- `7e8527a2`
- `7efb6e8b-c2cb-486a-9abe-457c6d6b8b9e`
- `7fd47de4-8a0f-4507-af7c-860a28a17cd5`
- `80a48f5c-e896-41d4-ba10-ddf4065b25a0`
- `815b65bf`
- `8915b4ab`
- `89ae011c`
- `89c4b74e-440b-485c-9b5f-73ae6aaac7aa`
- `8a32bdfe`
- `8a374d60`
- `8dcff2ac-facc-4bd6-86e2-d4b20231d493`
- `8e059c01`
- `927df905`
- `934c2d54`
- `93664445-016a-4ae3-b678-f49783980b4e`
- `98ea6160`
- `9b01ccd2`
- `9c147059-14ce-4a2f-8e44-e8498a3462e5`
- `9da906fb`
- `a5946680-912c-4394-89a8-40fd33723c20`
- `a689d6e5`
- `a7a051f8`
- `a928e734`
- `ab2244b3-5ada-470d-a713-77ae7de05eee`
- `b0c24049-ca8b-42c6-be14-4cb5f7c14efd`
- `b19d6516`
- `b3c4f438`
- `b5093d81-82cd-4775-8900-67444b5f172e`
- `b7027727`
- `b9213218`
- `bc6603d7-677c-400f-a596-055b041ed980`
- `bda79c54`
- `be03a0a4-f977-425a-b570-29904d0dc44a`
- `c214a92e`
- `c2eb6f88`
- `c439cc69`
- `c561d8a4`
- `ca889d5e`
- `cab7d83b`
- `cd897f36`
- `cf3c8ec6`
- `d4de195c-e97b-446e-939c-cf7245a41039`
- `d7c9cb88`
- `d86d1395-8999-4ffd-b72a-46d00db39d64`
- `e176480b`
- `e276960e`
- `e2965bef-7ce1-4410-aa82-6a4fdbf4d7a1`
- `e2d4c567-7aa9-4528-b4af-9b5129bbb4db`
- `e4a74cb0`
- `e566dcf0`
- `e93f6e01-c09c-4cb2-903a-881d16ae5efb`
- `eb91679e`
- `f9976f52`
- `fc1de089-766b-44ee-86f8-c0cbb534e178`
- `fdd276ac-6522-4d5c-8ed9-c40b1f5915ae`
- `feeeb219-1ae0-4282-a076-930fdb9611cd`
