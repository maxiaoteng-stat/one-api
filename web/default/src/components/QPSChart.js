import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Legend
} from 'recharts';
import { Card, Segment, Button, ButtonGroup } from 'semantic-ui-react';
import { API, showError } from '../helpers';

const QPSChart = () => {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState([]);
  const [timeUnit, setTimeUnit] = useState('second'); // 'second' 或 'minute'
  
  // 获取QPS数据
  const fetchQPSData = async () => {
    setLoading(true);
    try {
      // 修改API路径从/api/dashboard/qps到/api/rate-limit/qps
      const res = await API.get(`/api/rate-limit/qps?unit=${timeUnit}`);
      const { success, message, data } = res.data;
      
      if (success && data) {
        setData(data);
      } else {
        showError(message || t('general.error'));
      }
    } catch (error) {
      showError(error.message);
    } finally {
      setLoading(false);
    }
  };
  
  // 定时刷新数据
  useEffect(() => {
    fetchQPSData();
    
    const intervalId = setInterval(() => {
      fetchQPSData();
    }, timeUnit === 'second' ? 1000 : 10000); // 秒级数据每秒刷新，分钟级数据每10秒刷新
    
    return () => clearInterval(intervalId);
  }, [timeUnit]);
  
  // 切换时间单位
  const handleUnitChange = (unit) => {
    setTimeUnit(unit);
  };
  
  // 计算图表的X轴间隔
  const calculateXAxisInterval = () => {
    if (timeUnit === 'second') {
      return data.length > 30 ? 4 : 2; // 秒级数据，每5秒或3秒显示一个刻度
    } else {
      return data.length > 30 ? 9 : 4; // 分钟级数据，每10分钟或5分钟显示一个刻度
    }
  };
  
  // 格式化提示文本
  const formatTooltip = (value) => {
    return `${value} ${t('setting.rate_limit.qps_chart.requests_per_second')}`;
  };
  
  return (
    <Card fluid>
      <Card.Content>
        <Card.Header>
          {t('setting.rate_limit.qps_chart.title')}
          <ButtonGroup size='mini' floated='right'>
            <Button 
              primary={timeUnit === 'second'} 
              onClick={() => handleUnitChange('second')}
            >
              {t('setting.rate_limit.qps_chart.second')}
            </Button>
            <Button 
              primary={timeUnit === 'minute'} 
              onClick={() => handleUnitChange('minute')}
            >
              {t('setting.rate_limit.qps_chart.minute')}
            </Button>
          </ButtonGroup>
        </Card.Header>
        <Card.Description>
          <Segment loading={loading} style={{ height: '300px', padding: '1em 0' }}>
            {data.length > 0 ? (
              <ResponsiveContainer width="100%" height="100%">
                <LineChart
                  data={data}
                  margin={{ top: 5, right: 30, left: 20, bottom: 5 }}
                >
                  <CartesianGrid strokeDasharray="3 3" vertical={false} opacity={0.1} />
                  <XAxis 
                    dataKey="time" 
                    interval={calculateXAxisInterval()}
                    tick={{ fontSize: 12, fill: '#666' }}
                  />
                  <YAxis 
                    tick={{ fontSize: 12, fill: '#666' }}
                    allowDecimals={false}
                    minTickGap={1}
                  />
                  <Tooltip 
                    formatter={formatTooltip}
                    labelFormatter={(label) => `${label}`}
                    contentStyle={{
                      backgroundColor: '#fff',
                      border: '1px solid #ddd',
                      borderRadius: '4px',
                      padding: '10px'
                    }}
                  />
                  <Legend />
                  <Line 
                    type="monotone" 
                    dataKey="value" 
                    name={t('setting.rate_limit.qps_chart.qps')}
                    stroke="#4318FF" 
                    strokeWidth={2}
                    dot={false}
                    activeDot={{ r: 4 }}
                  />
                </LineChart>
              </ResponsiveContainer>
            ) : (
              <div style={{ 
                height: '100%', 
                display: 'flex', 
                alignItems: 'center', 
                justifyContent: 'center',
                color: '#999'
              }}>
                {t('setting.rate_limit.no_data')}
              </div>
            )}
          </Segment>
        </Card.Description>
      </Card.Content>
    </Card>
  );
};

export default QPSChart; 