import React, { useEffect, useState, useCallback } from 'react';
import {
  Title,
  Label,
  Pagination,
  Badge,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { safeFetch } from '../utils/api';

interface HistoryEntry {
  timestamp: string;
  agent: string;
  resource: string;
  namespace: string;
  cluster: string;
  action: string;
  beforeCPU: string;
  beforeMemory: string;
  afterCPU: string;
  afterMemory: string;
  status: string;
  aiResponse: string;
}

export const History: React.FC = () => {
  const [entries, setEntries] = useState<HistoryEntry[]>([]);
  const [page, setPage] = useState(1);
  const [perPage, setPerPage] = useState(10);

  const fetchHistory = useCallback(() => {
    const params = new URLSearchParams();
    params.set('limit', perPage.toString());
    params.set('offset', ((page - 1) * perPage).toString());
    safeFetch<HistoryEntry[]>(`/api/v1/history?${params.toString()}`)
      .then(d => setEntries(d || []));
  }, [page, perPage]);

  useEffect(() => {
    fetchHistory();
  }, [fetchHistory]);

  const statusColor = (status: string): 'green' | 'blue' | 'red' | 'grey' => {
    switch (status) {
      case 'applied': return 'green';
      case 'dry-run': return 'blue';
      case 'rejected': return 'red';
      default: return 'grey';
    }
  };

  return (
    <>
      <Title headingLevel="h1" size="2xl" style={{ marginBottom: '1rem' }}>
        Change History
      </Title>
      <p style={{ marginBottom: '1rem', color: '#6a737d' }}>
        Historical record of all resource changes made by Kairos agents.
      </p>

      <Pagination
        itemCount={entries.length < perPage ? (page - 1) * perPage + entries.length : page * perPage + 1}
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
            <Th>Cluster</Th>
            <Th>Action</Th>
            <Th>Before (CPU / Mem)</Th>
            <Th>After (CPU / Mem)</Th>
            <Th>Status</Th>
          </Tr>
        </Thead>
        <Tbody>
          {entries.length === 0 ? (
            <Tr>
              <Td colSpan={9} style={{ textAlign: 'center', padding: '2rem' }}>
                No history entries found.
              </Td>
            </Tr>
          ) : (
            entries.map((entry, idx) => (
              <Tr key={`${entry.timestamp}-${entry.resource}-${idx}`}>
                <Td>{new Date(entry.timestamp).toLocaleString()}</Td>
                <Td><Label color="purple">{entry.agent}</Label></Td>
                <Td>{entry.resource}</Td>
                <Td>{entry.namespace}</Td>
                <Td><Label color="cyan">{entry.cluster}</Label></Td>
                <Td>
                  <Badge>{entry.action}</Badge>
                </Td>
                <Td>
                  <code>{entry.beforeCPU || '-'}</code> / <code>{entry.beforeMemory || '-'}</code>
                </Td>
                <Td>
                  <code>{entry.afterCPU || '-'}</code> / <code>{entry.afterMemory || '-'}</code>
                </Td>
                <Td>
                  <Label color={statusColor(entry.status)}>{entry.status}</Label>
                </Td>
              </Tr>
            ))
          )}
        </Tbody>
      </Table>

      {entries.length >= perPage && (
        <Pagination
          itemCount={page * perPage + 1}
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
