import React, { useEffect, useState } from 'react';
import {
  Title,
  Label,
  Button,
  Card,
  CardBody,
  ExpandableSection,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';

interface ApprovalRequest {
  id: string;
  resource: string;
  namespace: string;
  cluster: string;
  agent: string;
  reason: string;
  status: string;
  action: string;
  timestamp: string;
  before: Record<string, string> | null;
  after: Record<string, string> | null;
  dryRun: boolean;
  policyName: string;
}

export const Approvals: React.FC = () => {
  const [approvals, setApprovals] = useState<ApprovalRequest[]>([]);

  const fetchApprovals = () => {
    fetch('/api/v1/approvals')
      .then(r => r.json())
      .then(data => setApprovals(data || []))
      .catch(() => setApprovals([]));
  };

  useEffect(() => {
    fetchApprovals();
  }, []);

  const handleAction = (id: string, action: 'approve' | 'reject') => {
    fetch(`/api/v1/approvals/${id}/${action}`, { method: 'POST' })
      .then(() => fetchApprovals())
      .catch(console.error);
  };

  const formatSnapshot = (snapshot: Record<string, string> | null): string => {
    if (!snapshot || Object.keys(snapshot).length === 0) return '-';
    return Object.entries(snapshot).map(([k, v]) => `${k}: ${v}`).join(', ');
  };

  return (
    <>
      <Title headingLevel="h1" size="2xl" style={{ marginBottom: '1rem' }}>
        Pending Approvals
      </Title>
      <p style={{ marginBottom: '1rem', color: '#6a737d' }}>
        Approval requests from agents running in supervised mode. Review proposed changes before they are applied.
      </p>

      <Card isFlat>
        <CardBody>
          <Table aria-label="Approvals table">
            <Thead>
              <Tr>
                <Th>Resource</Th>
                <Th>Namespace</Th>
                <Th>Cluster</Th>
                <Th>Agent</Th>
                <Th>Action</Th>
                <Th>Before / After</Th>
                <Th>Reason</Th>
                <Th>Actions</Th>
              </Tr>
            </Thead>
            <Tbody>
              {approvals.length === 0 ? (
                <Tr>
                  <Td colSpan={8} style={{ textAlign: 'center', padding: '2rem' }}>
                    No pending approval requests.
                  </Td>
                </Tr>
              ) : (
                approvals.map((req) => (
                  <Tr key={req.id}>
                    <Td>{req.resource}</Td>
                    <Td>{req.namespace}</Td>
                    <Td><Label color="blue">{req.cluster}</Label></Td>
                    <Td><Label color="purple">{req.agent}</Label></Td>
                    <Td><Label color="orange">{req.action}</Label></Td>
                    <Td>
                      <ExpandableSection toggleText="View changes" isIndented>
                        <div style={{ fontFamily: 'monospace', fontSize: '0.85rem' }}>
                          <div style={{ padding: '0.25rem 0.5rem', background: 'rgba(248, 81, 73, 0.1)', borderLeft: '3px solid #f85149', marginBottom: '4px' }}>
                            <strong style={{ color: '#f85149' }}>Before:</strong> {formatSnapshot(req.before)}
                          </div>
                          <div style={{ padding: '0.25rem 0.5rem', background: 'rgba(63, 185, 80, 0.1)', borderLeft: '3px solid #3fb950' }}>
                            <strong style={{ color: '#3fb950' }}>After:</strong> {formatSnapshot(req.after)}
                          </div>
                        </div>
                      </ExpandableSection>
                    </Td>
                    <Td style={{ maxWidth: '300px' }}>{req.reason}</Td>
                    <Td>
                      <Button
                        variant="primary"
                        size="sm"
                        style={{ marginRight: '0.5rem', backgroundColor: '#3e8635' }}
                        onClick={() => handleAction(req.id, 'approve')}
                      >
                        Approve
                      </Button>
                      <Button
                        variant="danger"
                        size="sm"
                        onClick={() => handleAction(req.id, 'reject')}
                      >
                        Reject
                      </Button>
                    </Td>
                  </Tr>
                ))
              )}
            </Tbody>
          </Table>
        </CardBody>
      </Card>
    </>
  );
};
