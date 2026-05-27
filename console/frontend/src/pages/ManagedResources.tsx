import React, { useEffect, useState } from 'react';
import {
  Title,
  Label,
  Pagination,
  Badge,
  Card,
  CardBody,
  Flex,
  FlexItem,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';

interface ManagedResource {
  name: string;
  namespace: string;
  kind: string;
  cluster: string;
  policy: string;
  agent: string;
  currentCPU: string;
  currentMemory: string;
  status: string;
}

export const ManagedResources: React.FC = () => {
  const [resources, setResources] = useState<ManagedResource[]>([]);
  const [page, setPage] = useState(1);
  const [perPage, setPerPage] = useState(10);
  const [clusterFilter, setClusterFilter] = useState<string>('all');

  useEffect(() => {
    fetch('/api/v1/managed-resources').then(r => r.json()).then(data => {
      setResources(data || []);
    });
  }, []);

  const clusters = Array.from(new Set(resources.map(r => r.cluster)));

  const filteredResources = clusterFilter === 'all'
    ? resources
    : resources.filter(r => r.cluster === clusterFilter);

  const startIdx = (page - 1) * perPage;
  const paginatedResources = filteredResources.slice(startIdx, startIdx + perPage);

  const clusterColor = (cluster: string): "blue" | "purple" | "green" | "orange" | "cyan" => {
    switch (cluster) {
      case 'hub': return 'blue';
      case 'east': return 'green';
      case 'west': return 'orange';
      default: return 'cyan';
    }
  };

  return (
    <>
      <Title headingLevel="h1" size="2xl" style={{ marginBottom: '1rem' }}>
        Managed Resources — Multi-Cluster View
      </Title>
      <p style={{ marginBottom: '1rem', color: '#6a737d' }}>
        Resources annotated with <code>kairos.io/managed: "true"</code> across all clusters.
        Hub resources are queried directly; spoke cluster resources are reported by their KairosAgents.
      </p>

      <Flex style={{ marginBottom: '1rem', gap: '0.5rem' }}>
        <FlexItem>
          <button
            onClick={() => { setClusterFilter('all'); setPage(1); }}
            style={{
              padding: '0.4rem 1rem',
              border: clusterFilter === 'all' ? '2px solid #06c' : '1px solid #444',
              borderRadius: '20px',
              background: clusterFilter === 'all' ? '#06c' : 'transparent',
              color: clusterFilter === 'all' ? '#fff' : '#ccc',
              cursor: 'pointer',
            }}
          >
            All <Badge>{resources.length}</Badge>
          </button>
        </FlexItem>
        {clusters.map(c => (
          <FlexItem key={c}>
            <button
              onClick={() => { setClusterFilter(c); setPage(1); }}
              style={{
                padding: '0.4rem 1rem',
                border: clusterFilter === c ? '2px solid #06c' : '1px solid #444',
                borderRadius: '20px',
                background: clusterFilter === c ? '#06c' : 'transparent',
                color: clusterFilter === c ? '#fff' : '#ccc',
                cursor: 'pointer',
              }}
            >
              {c} <Badge>{resources.filter(r => r.cluster === c).length}</Badge>
            </button>
          </FlexItem>
        ))}
      </Flex>

      <Card isFlat>
        <CardBody>
          <Pagination
            itemCount={filteredResources.length}
            perPage={perPage}
            page={page}
            onSetPage={(_e, p) => setPage(p)}
            onPerPageSelect={(_e, pp) => { setPerPage(pp); setPage(1); }}
            isCompact
            style={{ marginBottom: '1rem' }}
          />

          <Table aria-label="Managed Resources">
            <Thead>
              <Tr>
                <Th>Name</Th>
                <Th>Namespace</Th>
                <Th>Kind</Th>
                <Th>Cluster</Th>
                <Th>Policy</Th>
                <Th>Agent</Th>
                <Th>CPU</Th>
                <Th>Memory</Th>
                <Th>Status</Th>
              </Tr>
            </Thead>
            <Tbody>
              {paginatedResources.length === 0 ? (
                <Tr>
                  <Td colSpan={9} style={{ textAlign: 'center', padding: '2rem' }}>
                    No managed resources found. Ensure KairosAgents report to the hub or annotate workloads with <code>kairos.io/managed: "true"</code>.
                  </Td>
                </Tr>
              ) : (
                paginatedResources.map((res, idx) => (
                  <Tr key={`${res.cluster}-${res.namespace}-${res.name}-${idx}`}>
                    <Td>{res.name}</Td>
                    <Td>{res.namespace}</Td>
                    <Td><Label color="blue">{res.kind}</Label></Td>
                    <Td><Label color={clusterColor(res.cluster)}>{res.cluster}</Label></Td>
                    <Td>{res.policy || '-'}</Td>
                    <Td>{res.agent || '-'}</Td>
                    <Td><code>{res.currentCPU || '-'}</code></Td>
                    <Td><code>{res.currentMemory || '-'}</code></Td>
                    <Td><Label color="green">{res.status}</Label></Td>
                  </Tr>
                ))
              )}
            </Tbody>
          </Table>

          {filteredResources.length > perPage && (
            <Pagination
              itemCount={filteredResources.length}
              perPage={perPage}
              page={page}
              onSetPage={(_e, p) => setPage(p)}
              onPerPageSelect={(_e, pp) => { setPerPage(pp); setPage(1); }}
              variant="bottom"
              style={{ marginTop: '1rem' }}
            />
          )}
        </CardBody>
      </Card>
    </>
  );
};
