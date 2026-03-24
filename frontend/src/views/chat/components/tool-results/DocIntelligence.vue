<template>
  <!-- ───────────── doc_instruction ───────────── -->
  <div v-if="displayType === 'doc_instruction'" class="doc-intel-display">
    <div class="section-header">
      <t-icon name="edit" class="section-icon" />
      <span>{{ $t('docIntel.instructionTitle') }}</span>
    </div>

    <div v-if="instructionData" class="instruction-body">
      <div class="field-row">
        <span class="field-label">{{ $t('docIntel.instruction') }}:</span>
        <span class="field-value instruction-text">{{ instructionData.instruction }}</span>
      </div>

      <!-- Before / After side-by-side -->
      <div class="diff-container">
        <div class="diff-col">
          <div class="diff-header">{{ $t('docIntel.originalContent') }}</div>
          <pre class="diff-content original">{{ instructionData.original_content }}</pre>
        </div>
        <div class="diff-col">
          <div class="diff-header result-label">{{ $t('docIntel.resultContent') }}</div>
          <pre class="diff-content result">{{ instructionData.result_content }}</pre>
        </div>
      </div>

      <!-- Copy result button -->
      <div class="toolbar">
        <t-button size="small" variant="outline" shape="round" @click="copyText(instructionData.result_content)">
          <t-icon name="copy" />
          {{ $t('docIntel.copyResult') }}
        </t-button>
      </div>
    </div>
  </div>

  <!-- ───────────── doc_extract ───────────── -->
  <div v-else-if="displayType === 'doc_extract'" class="doc-intel-display">
    <div class="section-header">
      <t-icon name="list" class="section-icon" />
      <span>{{ $t('docIntel.extractTitle') }}</span>
    </div>

    <div v-if="extractData">
      <div v-if="extractData.fields && extractData.fields.length > 0" class="fields-table-wrapper">
        <table class="fields-table">
          <thead>
            <tr>
              <th>{{ $t('docIntel.fieldName') }}</th>
              <th>{{ $t('docIntel.fieldValue') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(field, idx) in extractData.fields" :key="idx">
              <td class="field-name-cell">{{ field.name }}</td>
              <td class="field-value-cell">{{ field.value }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-else class="no-fields">{{ $t('docIntel.noFieldsExtracted') }}</div>

      <!-- Copy JSON button -->
      <div class="toolbar">
        <t-button size="small" variant="outline" shape="round" @click="copyJSON(extractData.fields)">
          <t-icon name="copy" />
          {{ $t('docIntel.copyJSON') }}
        </t-button>
      </div>
    </div>
  </div>

  <!-- ───────────── doc_fill_table ───────────── -->
  <div v-else-if="displayType === 'doc_fill_table'" class="doc-intel-display">
    <div class="section-header">
      <t-icon name="table" class="section-icon" />
      <span>{{ $t('docIntel.fillTableTitle') }}</span>
      <span v-if="fillTableData && fillTableData.accuracy_hint > 0" class="accuracy-badge" :class="accuracyClass">
        {{ $t('docIntel.accuracy') }}: {{ Math.round(fillTableData.accuracy_hint * 100) }}%
      </span>
    </div>

    <div v-if="fillTableData">
      <!-- Filled table preview -->
      <div v-if="fillTableData.filled_fields && fillTableData.filled_fields.length > 0"
           class="fields-table-wrapper">
        <table class="fields-table filled-table">
          <thead>
            <tr>
              <th v-for="field in fillTableData.filled_fields" :key="field.name">{{ field.name }}</th>
            </tr>
          </thead>
          <tbody>
            <tr>
              <td v-for="field in fillTableData.filled_fields" :key="field.name" class="field-value-cell">
                {{ field.value || $t('docIntel.emptyCell') }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-else class="no-fields">{{ $t('docIntel.noFieldsFilled') }}</div>

      <!-- Download CSV button -->
      <div class="toolbar">
        <t-button size="small" variant="outline" shape="round" @click="copyJSON(fillTableData.filled_fields)">
          <t-icon name="copy" />
          {{ $t('docIntel.copyJSON') }}
        </t-button>
        <t-button
          v-if="fillTableData.csv_content"
          size="small"
          theme="primary"
          shape="round"
          @click="downloadCSV(fillTableData.csv_content)"
        >
          <t-icon name="download" />
          {{ $t('docIntel.downloadCSV') }}
        </t-button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue';
import { MessagePlugin } from 'tdesign-vue-next';
import { useI18n } from 'vue-i18n';
import type {
  DocInstructionData,
  DocExtractData,
  DocFillTableData,
  DisplayType,
} from '@/types/tool-results';

interface Props {
  displayType: DisplayType;
  data: DocInstructionData | DocExtractData | DocFillTableData;
}

const props = defineProps<Props>();
const { t } = useI18n();

// ─── Typed accessors ─────────────────────────────────────────────────────────

const instructionData = computed(() =>
  props.displayType === 'doc_instruction' ? (props.data as DocInstructionData) : null,
);

const extractData = computed(() =>
  props.displayType === 'doc_extract' ? (props.data as DocExtractData) : null,
);

const fillTableData = computed(() =>
  props.displayType === 'doc_fill_table' ? (props.data as DocFillTableData) : null,
);

// ─── Accuracy badge colour ────────────────────────────────────────────────────

const accuracyClass = computed(() => {
  const hint = fillTableData.value?.accuracy_hint ?? 0;
  if (hint >= 0.8) return 'accuracy-high';
  if (hint >= 0.6) return 'accuracy-medium';
  return 'accuracy-low';
});

// ─── Actions ─────────────────────────────────────────────────────────────────

const copyText = async (text: string) => {
  try {
    await navigator.clipboard.writeText(text);
    MessagePlugin.success(t('common.copySuccess'));
  } catch {
    MessagePlugin.error(t('common.copyFailed'));
  }
};

const copyJSON = async (data: unknown) => {
  await copyText(JSON.stringify(data, null, 2));
};

const downloadCSV = (csvContent: string) => {
  // Add UTF-8 BOM so Excel opens the file with the correct encoding.
  const bom = '\uFEFF';
  const blob = new Blob([bom + csvContent], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = 'filled_table.csv';
  document.body.appendChild(a);
  a.click();
  setTimeout(() => {
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  }, 200);
};
</script>

<style lang="less" scoped>
.doc-intel-display {
  font-size: 13px;
  color: var(--td-text-color-primary);
}

.section-header {
  display: flex;
  align-items: center;
  gap: 6px;
  font-weight: 600;
  font-size: 13px;
  color: var(--td-text-color-primary);
  margin-bottom: 12px;

  .section-icon {
    font-size: 16px;
    color: var(--td-brand-color);
  }
}

.accuracy-badge {
  margin-left: auto;
  font-size: 11px;
  font-weight: 500;
  padding: 2px 8px;
  border-radius: 999px;

  &.accuracy-high {
    background: var(--td-success-color-light);
    color: var(--td-success-color);
  }
  &.accuracy-medium {
    background: var(--td-warning-color-light);
    color: var(--td-warning-color);
  }
  &.accuracy-low {
    background: var(--td-error-color-light);
    color: var(--td-error-color);
  }
}

/* Instruction display */
.instruction-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.field-row {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;

  .field-label {
    font-weight: 500;
    white-space: nowrap;
  }
  .instruction-text {
    color: var(--td-text-color-secondary);
  }
}

.diff-container {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;

  @media (max-width: 600px) {
    grid-template-columns: 1fr;
  }
}

.diff-col {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.diff-header {
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--td-text-color-placeholder);

  &.result-label {
    color: var(--td-brand-color);
  }
}

.diff-content {
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-border);
  border-radius: 6px;
  padding: 10px 12px;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 260px;
  overflow-y: auto;
  margin: 0;

  &.result {
    border-color: var(--td-brand-color-light);
    background: var(--td-brand-color-light);
  }
}

/* Fields / extraction table */
.fields-table-wrapper {
  overflow-x: auto;
  border-radius: 6px;
  border: 1px solid var(--td-component-border);
  margin-bottom: 10px;
}

.fields-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;

  th {
    background: var(--td-bg-color-secondarycontainer);
    padding: 8px 12px;
    text-align: left;
    font-weight: 600;
    color: var(--td-text-color-primary);
    white-space: nowrap;
  }

  td {
    padding: 7px 12px;
    border-top: 1px solid var(--td-component-border);
    vertical-align: top;
    word-break: break-word;
  }

  tr:hover td {
    background: var(--td-bg-color-container-hover);
  }
}

/* When rendering as a horizontal filled table (one data row) */
.filled-table {
  th, td {
    max-width: 160px;
    min-width: 80px;
  }
}

.field-name-cell {
  font-weight: 500;
  white-space: nowrap;
  color: var(--td-text-color-secondary);
  width: 30%;
}

.field-value-cell {
  color: var(--td-text-color-primary);
}

.no-fields {
  color: var(--td-text-color-placeholder);
  font-size: 12px;
  padding: 8px 0;
}

/* Toolbar */
.toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 8px;
}
</style>
