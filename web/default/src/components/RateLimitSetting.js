import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Form,
  Button,
  Segment,
  Header
} from 'semantic-ui-react';
import { API, showError, showSuccess } from '../helpers';
import QPSChart from './QPSChart';

const RateLimitSetting = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [inputs, setInputs] = useState(null);

  // 处理输入变化，确保数字字段为数字类型
  const handleInputChange = (e, { name, value }) => {
    const numericFields = ['max_qps', 'queue_capacity', 'queue_timeout', 'daily_quota', 'warning_threshold'];
    const processedValue = numericFields.includes(name) ? (value === '' ? 0 : Number(value)) : value;
    
    setInputs(prev => ({ ...prev, [name]: processedValue }));
  };

  // 获取设置数据
  const fetchSettings = async () => {
    setLoading(true);
    try {
      const res = await API.get('/api/rate-limit/global');
      const { success, message, data } = res.data;
      
      if (success && data) {
        // 确保所有数字字段都是数字类型
        const numericFields = ['max_qps', 'queue_capacity', 'queue_timeout', 'daily_quota', 'warning_threshold'];
        const formattedData = { ...data };
        
        numericFields.forEach(field => {
          formattedData[field] = Number(data[field] || 0);
        });
        
        if (!formattedData.dingtalk_webhook) formattedData.ding_talk_webhook = '';
        
        setInputs(formattedData);
      } else {
        showError(message || t('general.error'));
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setLoading(false);
    }
  };

  // 提交表单
  const handleSubmit = async () => {
    if (!inputs) return;
    
    setSaving(true);
    try {
      const res = await API.put('/api/rate-limit/global', inputs);
      const { success, message } = res.data;
      
      if (success) {
        showSuccess(t('setting.rate_limit.update_success'));
      } else {
        showError(message || t('setting.rate_limit.update_failed'));
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setSaving(false);
    }
  };

  // 组件挂载时获取设置
  useEffect(() => {
    fetchSettings();
  }, []);

  // 数据加载中时显示加载状态
  if (!inputs) {
    return <Segment loading={true} style={{ minHeight: '200px' }} />;
  }

  return (
    <>
      {/* 添加QPS图表在限流设置上方 */}
      <QPSChart />
      
      <Segment loading={loading}>
        <Header as='h3'>{t('setting.rate_limit.title')}</Header>
        <Form>
          <Form.Field>
            <label>{t('setting.rate_limit.enabled')}</label>
            <Form.Checkbox
              toggle
              name='enabled'
              checked={inputs.enabled}
              onChange={(e, { name, checked }) => handleInputChange(e, { name, value: checked })}
            />
            <small>{t('setting.rate_limit.enabled_hint')}</small>
          </Form.Field>
          <Form.Field>
            <label>{t('setting.rate_limit.max_qps')}</label>
            <Form.Input
              name='max_qps'
              type='number'
              min={0}
              placeholder={t('setting.rate_limit.max_qps_placeholder')}
              onChange={handleInputChange}
              value={inputs.max_qps}
            />
            <small>{t('setting.rate_limit.max_qps_hint')}</small>
          </Form.Field>
          <Form.Field>
            <label>{t('setting.rate_limit.daily_quota')}</label>
            <Form.Input
              name='daily_quota'
              type='number'
              min={0}
              placeholder={t('setting.rate_limit.daily_quota_placeholder')}
              onChange={handleInputChange}
              value={inputs.daily_quota}
            />
            <small>{t('setting.rate_limit.daily_quota_hint')}</small>
          </Form.Field>
          <Form.Field>
            <label>{t('setting.rate_limit.queue_capacity')}</label>
            <Form.Input
              name='queue_capacity'
              type='number'
              min={0}
              placeholder={t('setting.rate_limit.queue_capacity_placeholder')}
              onChange={handleInputChange}
              value={inputs.queue_capacity}
            />
            <small>{t('setting.rate_limit.queue_capacity_hint')}</small>
          </Form.Field>
          <Form.Field>
            <label>{t('setting.rate_limit.queue_timeout')}</label>
            <Form.Input
              name='queue_timeout'
              type='number'
              min={1}
              placeholder={t('setting.rate_limit.queue_timeout_placeholder')}
              onChange={handleInputChange}
              value={inputs.queue_timeout}
            />
            <small>{t('setting.rate_limit.queue_timeout_hint')}</small>
          </Form.Field>
          <Form.Field>
            <label>{t('setting.rate_limit.warning_threshold')}</label>
            <Form.Input
              name='warning_threshold'
              type='number'
              min={0}
              max={100}
              placeholder={t('setting.rate_limit.warning_threshold_placeholder')}
              onChange={handleInputChange}
              value={inputs.warning_threshold}
            />
            <small>{t('setting.rate_limit.warning_threshold_hint')}</small>
          </Form.Field>
          <Form.Field>
            <label>{t('setting.rate_limit.dingtalk_webhook')}</label>
            <Form.Input
              name='dingtalk_webhook'
              placeholder={t('setting.rate_limit.dingtalk_webhook_placeholder')}
              onChange={handleInputChange}
              value={inputs.dingtalk_webhook}
            />
            <small>{t('setting.rate_limit.dingtalk_webhook_hint')}</small>
          </Form.Field>
          <Button type='button' onClick={handleSubmit} loading={saving} primary>
            {t('setting.rate_limit.save')}
          </Button>
        </Form>
      </Segment>
    </>
  );
};

export default RateLimitSetting;
