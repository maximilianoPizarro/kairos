import React, { useEffect, useState } from 'react';
import {
  Title,
  Label,
  Pagination,
  Badge,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';

interface HistoryEntry {
  timestamp: string;
  agent: string;
  resource: string;
  namespace: string;
  action: string;
  beforeCPU: string;
  beforeMemory: string;
  afterCPU: string;
  afterMemory: string;
  status: string;
}

export const History: React.FC = () => {
  const [entries, setEntries] = useState<HistoryEntry[]>([]);
  const [page, setPage] = useState(1);
  const [perPage, setPerPage] = useState(10);

  useEffect(() => {
    fetch('/api/v1/history')
      .then(r => r.json())
      .then(data => setEntries(data || []))
      .catch(() => setEntries([]));
  }, []);

  const statusColor = (status: string): 'green' | 'blue' | 'red' | 'grey' => {
    switch (status) {
      case 'applied': return 'green';
      case 'dry-run': return 'blue';
      case 'rejected': return 'red';
      default: return 'grey';
    }
  };

  const startIdx = (page - 1) * perPage;
  const paginatedEntries = entries.slice(startIdx, startIdx + perPage);

  return (
    <>
      <Title headingLevel="h1" size="2xl" style={{ marginBottom: '1rem' }}>
        Change History
      </Title>
      <p style={{ marginBottom: '1rem', color: '#6a737d' }}>
        Historical record of all resource changes made by Kairos agents.
      </p>

      <Pagination
        itemCount={entries.length}
        perPage={perPage}
        page={page}
        onSetPage={(_e, p) => setPage(p)}
        onPerPageSelect={(_e, pp) => { setPerPage(pp); setPage(1); }}
        style={{ marginBottom: '1rem' }}
      />

      <Table aria-label="History table">
        <Thead>
          <Tr>
            <Th>Timestamp</Th>
            <Th>Agent</Th>
            <Th>Resource</Th>
            <Th>Namespace</Th>
            <Th>Action</Th>
            <Th>Before (CPU / Mem)</Th>
            <Th>After (CPU / Mem)</Th>
            <Th>Status</Th>
          </Tr>
        </Thead>
        <Tbody>
          {paginatedEntries.length === 0 ? (
            <Tr>
              <Td colSpan={8} style={{ textAlign: 'center', padding: '2rem' }}>
                No history entries found.
              </Td>
            </Tr>
          ) : (
            paginatedEntries.map((entry, idx) => (
              <Tr key={`${entry.timestamp}-${entry.resource}-${idx}`}>
                <Td>{new Date(entry.timestamp).toLocaleString()}</Td>
                <Td><Label color="purple">{entry.agent}</Label></Td>
                <Td>{entry.resource}</Td>
                <Td>{entry.namespace}</Td>
                <Td>
                  <Badge>{entry.action}</Badge>
                </Td>
                <Td>
                  <code>{entry.beforeCPU}</code> / <code>{entry.beforeMemory}</code>
                </Td>
                <Td>
                  <code>{entry.afterCPU}</code> / <code>{entry.afterMemory}</code>
                </Td>
                <Td>
                  <Label color={statusColor(entry.status)}>{entry.status}</Label>
                </Td>
              </Tr>
            ))
          )}
        </Tbody>
      </Table>

      {entries.length > perPage && (
        <Pagination
          itemCount={entries.length}
          perPage={perPage}
          page={page}
          onSetPage={(_e, p) => setPage(p)}
          onPerPageSelect={(_e, pp) => { setPerPage(pp); setPage(1); }}
          variant="bottom"
          style={{ marginTop: '1rem' }}
        />
      )}
    </>
  );
};
