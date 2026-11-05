-- +goose Up
-- Every identifier of the schema in English: the table `alertas` and the
-- Portuguese columns, indexes, constraint and sequence that came with it.
--
-- Renames only, so no data moves and each statement is a catalog change. They
-- take a brief ACCESS EXCLUSIVE lock, and a pod still running the previous
-- release asks for the old names: run this with the old pods stopped (see the
-- engine changelog), not in the middle of a rolling update.
--
-- Not renamed here, on purpose: the JSONB keys inside task_runs.stages and
-- workflows.definition. Those are data, readers accept both spellings, and
-- rewriting a table of that size is not a rename.

ALTER TABLE alertas RENAME TO alerts;
ALTER SEQUENCE alertas_id_seq RENAME TO alerts_id_seq;
ALTER TABLE alerts RENAME CONSTRAINT alertas_pkey TO alerts_pkey;
ALTER TABLE alerts RENAME CONSTRAINT alertas_run_id_fkey TO alerts_run_id_fkey;

ALTER TABLE alerts RENAME COLUMN tipo TO kind;
ALTER TABLE alerts RENAME COLUMN canal TO channel;
ALTER TABLE alerts RENAME COLUMN disponivel_em TO available_at;
ALTER TABLE alerts RENAME COLUMN reivindicado_em TO claimed_at;
ALTER TABLE alerts RENAME COLUMN reivindicado_por TO claimed_by;
ALTER TABLE alerts RENAME COLUMN tentativas TO attempts;
ALTER TABLE alerts RENAME COLUMN erro TO error;
ALTER TABLE alerts RENAME COLUMN entregue_em TO delivered_at;
ALTER TABLE alerts RENAME COLUMN desistiu_em TO gave_up_at;
ALTER TABLE alerts RENAME COLUMN criado_em TO created_at;

ALTER TABLE load_metrics RENAME COLUMN em TO run_created_at;
ALTER TABLE load_metrics RENAME COLUMN linhas TO rows;
ALTER TABLE load_metrics RENAME COLUMN registros TO records;
ALTER TABLE load_metrics RENAME COLUMN ignorados TO ignored;
ALTER TABLE load_metrics RENAME COLUMN bytes_saida TO bytes_out;
ALTER TABLE load_metrics RENAME COLUMN bytes_entrada TO bytes_in;
ALTER TABLE load_metrics RENAME COLUMN paginas TO pages;
ALTER TABLE load_metrics RENAME COLUMN tentativas TO attempts;

ALTER TABLE queue_items RENAME COLUMN prioridade TO priority;
ALTER TABLE queue_items RENAME COLUMN disponivel_em TO available_at;
ALTER TABLE queue_items RENAME COLUMN reivindicado_em TO claimed_at;
ALTER TABLE queue_items RENAME COLUMN reivindicado_por TO claimed_by;
ALTER TABLE queue_items RENAME COLUMN criado_em TO created_at;
ALTER TABLE queue_items RENAME COLUMN renovado_em TO renewed_at;

ALTER TABLE runs RENAME COLUMN definicao TO definition;
ALTER TABLE runs RENAME COLUMN erro TO error;
ALTER TABLE runs RENAME COLUMN criado_em TO created_at;
ALTER TABLE runs RENAME COLUMN iniciado_em TO started_at;
ALTER TABLE runs RENAME COLUMN terminado_em TO finished_at;
ALTER TABLE runs RENAME COLUMN max_ativos TO max_active;
ALTER TABLE runs RENAME COLUMN grupo_concorrencia TO concurrency_group;

ALTER TABLE schedules RENAME COLUMN ativo TO active;
ALTER TABLE schedules RENAME COLUMN ultimo_slot TO last_slot;
ALTER TABLE schedules RENAME COLUMN criado_em TO created_at;
ALTER TABLE schedules RENAME COLUMN atualizado_em TO updated_at;

ALTER TABLE task_runs RENAME COLUMN erro TO error;
ALTER TABLE task_runs RENAME COLUMN iniciado_em TO started_at;
ALTER TABLE task_runs RENAME COLUMN terminado_em TO finished_at;
ALTER TABLE task_runs RENAME COLUMN etapas TO stages;
ALTER TABLE task_runs RENAME COLUMN sdk_versao TO sdk_version;
ALTER TABLE task_runs RENAME COLUMN saida TO output;

ALTER TABLE workflows RENAME COLUMN definicao TO definition;

ALTER INDEX alertas_pendentes_idx RENAME TO alerts_pending_idx;
ALTER INDEX alertas_por_run_idx RENAME TO alerts_by_run_idx;
ALTER INDEX queue_items_reivindicados_idx RENAME TO queue_items_claimed_idx;
ALTER INDEX queue_pendentes_idx RENAME TO queue_pending_idx;
ALTER INDEX runs_workflow_criado_idx RENAME TO runs_workflow_created_idx;
ALTER INDEX schedules_ativos_idx RENAME TO schedules_active_idx;

-- The comments the old migrations carried in Portuguese, restated in English so
-- a database that already ran them ends with the same text as a fresh one.
COMMENT ON TABLE alerts IS
  'Alert outbox. The row is written in the SAME transaction as the failure that justifies it.';
COMMENT ON TABLE load_metrics IS
  'The numbers every SDK load already produced, kept for trend reads. Derived from task_runs.stages.';
COMMENT ON COLUMN queue_items.renewed_at IS
  'Last renewal of the claim by its owner; NULL = none since the claim.';
COMMENT ON COLUMN runs.max_active IS
  'Simultaneous runs allowed for this workflow. 0 = no limit.';
COMMENT ON COLUMN runs.auto_params IS
  'Automatic parameters of the run: adjusted_at, interval, delay, previous_error. A snapshot taken when the attempt started.';
COMMENT ON COLUMN runs.concurrency_group IS
  'The run''s concurrency group; NULL = the workflow contends only with itself.';
COMMENT ON COLUMN task_runs.map_index IS
  'Index of the instance of a mapped step (for_each). -1 = unmapped step.';
COMMENT ON COLUMN task_runs.handover IS
  'Where the step keeps running after being handed over at shutdown; NULL = not handed over.';
COMMENT ON INDEX runs_workflow_created_idx IS
  'Console reads per workflow: the latest run, recent runs, the workflow page.';

-- +goose Down

ALTER INDEX schedules_active_idx RENAME TO schedules_ativos_idx;
ALTER INDEX runs_workflow_created_idx RENAME TO runs_workflow_criado_idx;
ALTER INDEX queue_pending_idx RENAME TO queue_pendentes_idx;
ALTER INDEX queue_items_claimed_idx RENAME TO queue_items_reivindicados_idx;
ALTER INDEX alerts_by_run_idx RENAME TO alertas_por_run_idx;
ALTER INDEX alerts_pending_idx RENAME TO alertas_pendentes_idx;

ALTER TABLE workflows RENAME COLUMN definition TO definicao;

ALTER TABLE task_runs RENAME COLUMN output TO saida;
ALTER TABLE task_runs RENAME COLUMN sdk_version TO sdk_versao;
ALTER TABLE task_runs RENAME COLUMN stages TO etapas;
ALTER TABLE task_runs RENAME COLUMN finished_at TO terminado_em;
ALTER TABLE task_runs RENAME COLUMN started_at TO iniciado_em;
ALTER TABLE task_runs RENAME COLUMN error TO erro;

ALTER TABLE schedules RENAME COLUMN updated_at TO atualizado_em;
ALTER TABLE schedules RENAME COLUMN created_at TO criado_em;
ALTER TABLE schedules RENAME COLUMN last_slot TO ultimo_slot;
ALTER TABLE schedules RENAME COLUMN active TO ativo;

ALTER TABLE runs RENAME COLUMN concurrency_group TO grupo_concorrencia;
ALTER TABLE runs RENAME COLUMN max_active TO max_ativos;
ALTER TABLE runs RENAME COLUMN finished_at TO terminado_em;
ALTER TABLE runs RENAME COLUMN started_at TO iniciado_em;
ALTER TABLE runs RENAME COLUMN created_at TO criado_em;
ALTER TABLE runs RENAME COLUMN error TO erro;
ALTER TABLE runs RENAME COLUMN definition TO definicao;

ALTER TABLE queue_items RENAME COLUMN renewed_at TO renovado_em;
ALTER TABLE queue_items RENAME COLUMN created_at TO criado_em;
ALTER TABLE queue_items RENAME COLUMN claimed_by TO reivindicado_por;
ALTER TABLE queue_items RENAME COLUMN claimed_at TO reivindicado_em;
ALTER TABLE queue_items RENAME COLUMN available_at TO disponivel_em;
ALTER TABLE queue_items RENAME COLUMN priority TO prioridade;

ALTER TABLE load_metrics RENAME COLUMN attempts TO tentativas;
ALTER TABLE load_metrics RENAME COLUMN pages TO paginas;
ALTER TABLE load_metrics RENAME COLUMN bytes_in TO bytes_entrada;
ALTER TABLE load_metrics RENAME COLUMN bytes_out TO bytes_saida;
ALTER TABLE load_metrics RENAME COLUMN ignored TO ignorados;
ALTER TABLE load_metrics RENAME COLUMN records TO registros;
ALTER TABLE load_metrics RENAME COLUMN rows TO linhas;
ALTER TABLE load_metrics RENAME COLUMN run_created_at TO em;

ALTER TABLE alerts RENAME COLUMN created_at TO criado_em;
ALTER TABLE alerts RENAME COLUMN gave_up_at TO desistiu_em;
ALTER TABLE alerts RENAME COLUMN delivered_at TO entregue_em;
ALTER TABLE alerts RENAME COLUMN error TO erro;
ALTER TABLE alerts RENAME COLUMN attempts TO tentativas;
ALTER TABLE alerts RENAME COLUMN claimed_by TO reivindicado_por;
ALTER TABLE alerts RENAME COLUMN claimed_at TO reivindicado_em;
ALTER TABLE alerts RENAME COLUMN available_at TO disponivel_em;
ALTER TABLE alerts RENAME COLUMN channel TO canal;
ALTER TABLE alerts RENAME COLUMN kind TO tipo;

ALTER TABLE alerts RENAME CONSTRAINT alerts_run_id_fkey TO alertas_run_id_fkey;
ALTER TABLE alerts RENAME CONSTRAINT alerts_pkey TO alertas_pkey;
ALTER SEQUENCE alerts_id_seq RENAME TO alertas_id_seq;
ALTER TABLE alerts RENAME TO alertas;
