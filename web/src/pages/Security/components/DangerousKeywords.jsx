import React, { useEffect, useState, useCallback } from 'react';
import {
  Button,
  Modal,
  Form,
  Tag,
  Switch,
  Space,
  Popconfirm,
  Input,
  Select,
  Spin,
  Empty,
  Toast,
} from '@douyinfe/semi-ui';
import {
  IllustrationNoResult,
} from '@douyinfe/semi-illustrations';
import { Plus, Search } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { API } from '../../../helpers/api';
import CardTable from '../../../components/common/ui/CardTable';

const severityColorMap = {
  high: 'red',
  medium: 'orange',
  low: 'yellow',
};

const severityLabelMap = {
  high: '高危',
  medium: '中危',
  low: '低危',
};

const actionColorMap = {
  ban_user: 'red',
  block_only: 'orange',
};

const actionLabelMap = {
  ban_user: '禁用用户',
  block_only: '仅拦截',
};

const matchTypeLabelMap = {
  exact: '精确匹配',
  regex: '正则匹配',
};

const checkScopeLabelMap = {
  user_only:     '用户消息',
  user_and_tool: '用户+工具',
  all:           '全量消息',
};

const DangerousKeywords = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [keywords, setKeywords] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [searchKeyword, setSearchKeyword] = useState('');
  const [filterSeverity, setFilterSeverity] = useState('');
  const [filterAction, setFilterAction] = useState('');
  const [modalVisible, setModalVisible] = useState(false);
  const [editingItem, setEditingItem] = useState(null);
  const [submitting, setSubmitting] = useState(false);

  const fetchKeywords = useCallback(async () => {
    setLoading(true);
    try {
      const params = new URLSearchParams({
        page: String(page),
        page_size: String(pageSize),
      });
      if (searchKeyword) params.set('keyword', searchKeyword);
      if (filterSeverity) params.set('severity', filterSeverity);
      if (filterAction) params.set('action', filterAction);

      const res = await API.get(`/api/security/keywords?${params}`);
      if (res.data.success) {
        setKeywords(res.data.data || []);
        setTotal(res.data.total || 0);
      }
    } catch (err) {
      Toast.error(t('获取关键词列表失败'));
    }
    setLoading(false);
  }, [page, pageSize, searchKeyword, filterSeverity, filterAction, t]);

  useEffect(() => {
    fetchKeywords();
  }, [fetchKeywords]);

  const handleCreate = () => {
    setEditingItem(null);
    setModalVisible(true);
  };

  const handleEdit = (record) => {
    setEditingItem(record);
    setModalVisible(true);
  };

  const handleDelete = async (id) => {
    try {
      const res = await API.delete(`/api/security/keywords/${id}`);
      if (res.data.success) {
        Toast.success(t('删除成功'));
        fetchKeywords();
      } else {
        Toast.error(res.data.message || t('删除失败'));
      }
    } catch {
      Toast.error(t('删除失败'));
    }
  };

  const handleToggle = async (id) => {
    try {
      const res = await API.post(`/api/security/keywords/${id}/toggle`);
      if (res.data.success) {
        fetchKeywords();
      }
    } catch {
      Toast.error(t('操作失败'));
    }
  };

  const handleSubmit = async (values) => {
    setSubmitting(true);
    try {
      let res;
      if (editingItem) {
        res = await API.put(`/api/security/keywords/${editingItem.id}`, values);
      } else {
        res = await API.post('/api/security/keywords', values);
      }
      if (res.data.success) {
        Toast.success(editingItem ? t('更新成功') : t('创建成功'));
        setModalVisible(false);
        fetchKeywords();
      } else {
        Toast.error(res.data.message || t('操作失败'));
      }
    } catch {
      Toast.error(t('操作失败'));
    }
    setSubmitting(false);
  };

  const columns = [
    {
      title: t('关键词'),
      dataIndex: 'keyword',
      width: 200,
      render: (text) => (
        <span style={{ wordBreak: 'break-all' }}>
          {text && text.length > 50 ? text.slice(0, 50) + '...' : text}
        </span>
      ),
    },
    {
      title: t('匹配类型'),
      dataIndex: 'match_type',
      width: 100,
      render: (val) => (
        <Tag size='small' color={val === 'regex' ? 'blue' : 'grey'}>
          {matchTypeLabelMap[val] || val}
        </Tag>
      ),
    },
    {
      title: t('检查范围'),
      dataIndex: 'check_scope',
      width: 120,
      render: (val) => (
        <Tag size='small' color={val === 'all' ? 'orange' : val === 'user_and_tool' ? 'blue' : 'green'}>
          {checkScopeLabelMap[val] || val}
        </Tag>
      ),
    },
    {
      title: t('危险等级'),
      dataIndex: 'severity',
      width: 80,
      render: (val) => (
        <Tag size='small' color={severityColorMap[val]}>
          {severityLabelMap[val] || val}
        </Tag>
      ),
    },
    {
      title: t('触发动作'),
      dataIndex: 'action',
      width: 100,
      render: (val) => (
        <Tag size='small' color={actionColorMap[val]}>
          {actionLabelMap[val] || val}
        </Tag>
      ),
    },
    {
      title: t('触发次数'),
      dataIndex: 'trigger_count',
      width: 80,
      align: 'center',
    },
    {
      title: t('通知管理员'),
      dataIndex: 'notify_admin',
      width: 90,
      align: 'center',
      render: (val) => (val ? '✓' : '—'),
    },
    {
      title: t('状态'),
      dataIndex: 'enabled',
      width: 80,
      render: (val, record) => (
        <Switch
          size='small'
          checked={val}
          onChange={() => handleToggle(record.id)}
        />
      ),
    },
    {
      title: t('操作'),
      dataIndex: 'operate',
      fixed: 'right',
      width: 140,
      render: (_, record) => (
        <Space>
          <Button
            theme='borderless'
            size='small'
            onClick={() => handleEdit(record)}
          >
            {t('编辑')}
          </Button>
          <Popconfirm
            title={t('确定删除该关键词吗？')}
            onConfirm={() => handleDelete(record.id)}
          >
            <Button theme='borderless' type='danger' size='small'>
              {t('删除')}
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div>
      <div
        style={{
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          marginBottom: 16,
          flexWrap: 'wrap',
          gap: 8,
        }}
      >
        <Space>
          <Input
            prefix={<Search size={14} />}
            placeholder={t('搜索关键词')}
            value={searchKeyword}
            onChange={(val) => {
              setSearchKeyword(val);
              setPage(1);
            }}
            showClear
            style={{ width: 200 }}
          />
          <Select
            placeholder={t('危险等级')}
            value={filterSeverity}
            onChange={(val) => {
              setFilterSeverity(val || '');
              setPage(1);
            }}
            showClear
            style={{ width: 120 }}
            optionList={[
              { label: t('高危'), value: 'high' },
              { label: t('中危'), value: 'medium' },
              { label: t('低危'), value: 'low' },
            ]}
          />
          <Select
            placeholder={t('触发动作')}
            value={filterAction}
            onChange={(val) => {
              setFilterAction(val || '');
              setPage(1);
            }}
            showClear
            style={{ width: 120 }}
            optionList={[
              { label: t('禁用用户'), value: 'ban_user' },
              { label: t('仅拦截'), value: 'block_only' },
            ]}
          />
        </Space>
        <Button
          icon={<Plus size={14} />}
          theme='solid'
          type='primary'
          onClick={handleCreate}
        >
          {t('新建关键词')}
        </Button>
      </div>

      <Spin spinning={loading}>
        <CardTable
          columns={columns}
          dataSource={keywords}
          scroll={{ x: 'max-content' }}
          pagination={{
            currentPage: page,
            pageSize: pageSize,
            total: total,
            pageSizeOpts: [10, 20, 50],
            showSizeChanger: true,
            onPageSizeChange: (newSize) => {
              setPageSize(newSize);
              setPage(1);
            },
            onPageChange: (newPage) => setPage(newPage),
          }}
          empty={
            <Empty
              image={
                <IllustrationNoResult style={{ width: 150, height: 150 }} />
              }
              description={t('暂无危险关键词')}
            />
          }
          size='middle'
        />
      </Spin>

      <Modal
        title={editingItem ? t('编辑关键词') : t('新建关键词')}
        visible={modalVisible}
        onCancel={() => setModalVisible(false)}
        footer={null}
        width={520}
      >
        <Form
          initValues={
            editingItem || {
              keyword: '',
              match_type: 'exact',
              check_scope: 'user_only',
              severity: 'high',
              action: 'ban_user',
              notify_admin: true,
              enabled: true,
              description: '',
            }
          }
          onSubmit={handleSubmit}
          labelPosition='left'
          labelWidth={100}
        >
          <Form.TextArea
            field='keyword'
            label={t('关键词')}
            placeholder={t('输入关键词')}
            rules={[{ required: true, message: t('关键词不能为空') }]}
            autosize={{ minRows: 2, maxRows: 6 }}
          />
          <Form.RadioGroup field='match_type' label={t('匹配类型')}>
            <Form.Radio value='exact'>{t('精确匹配')}</Form.Radio>
            <Form.Radio value='regex'>{t('正则匹配')}</Form.Radio>
          </Form.RadioGroup>
          <Form.Select
            field='check_scope'
            label={t('检查范围')}
            optionList={[
              { label: t('仅用户消息（推荐·含历史轮次）'), value: 'user_only' },
              { label: t('用户消息+工具读取内容（防文档泄露）'), value: 'user_and_tool' },
              { label: t('全量消息（最严格·性能开销最大）'), value: 'all' },
            ]}
          />
          <Form.RadioGroup field='severity' label={t('危险等级')}>
            <Form.Radio value='high'>{t('高危')}</Form.Radio>
            <Form.Radio value='medium'>{t('中危')}</Form.Radio>
            <Form.Radio value='low'>{t('低危')}</Form.Radio>
          </Form.RadioGroup>
          <Form.Select
            field='action'
            label={t('触发动作')}
            optionList={[
              { label: t('禁用用户'), value: 'ban_user' },
              { label: t('仅拦截'), value: 'block_only' },
            ]}
          />
          <Form.Switch field='notify_admin' label={t('通知管理员')} />
          <Form.Switch field='enabled' label={t('启用')} />
          <Form.TextArea
            field='description'
            label={t('备注')}
            placeholder={t('可选备注说明')}
            autosize={{ minRows: 2, maxRows: 4 }}
          />
          <div style={{ textAlign: 'right', marginTop: 16 }}>
            <Space>
              <Button onClick={() => setModalVisible(false)}>
                {t('取消')}
              </Button>
              <Button
                theme='solid'
                type='primary'
                htmlType='submit'
                loading={submitting}
              >
                {editingItem ? t('更新') : t('创建')}
              </Button>
            </Space>
          </div>
        </Form>
      </Modal>
    </div>
  );
};

export default DangerousKeywords;
