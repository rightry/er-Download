import React, { useEffect } from 'react';
import { useDispatch, useSelector } from 'react-redux';
import {
  Box,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  IconButton,
  LinearProgress,
  Typography,
} from '@mui/material';
import {
  Pause as PauseIcon,
  PlayArrow as ResumeIcon,
  Delete as DeleteIcon,
} from '@mui/icons-material';
import AddDownloadButton from './AddDownloadButton';
import { 
  fetchDownloads, 
  startDownload,
  pauseDownload, 
  resumeDownload, 
  cancelDownload 
} from '../../store/slices/downloadsSlice';

// 格式化文件大小
const formatSize = (bytes) => {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
};

// 格式化速度
const formatSpeed = (bytesPerSecond) => {
  return formatSize(bytesPerSecond) + '/s';
};

// 格式化剩余时间
const formatRemainingTime = (seconds) => {
  if (seconds < 60) return `${seconds}秒`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}分钟`;
  return `${Math.floor(seconds / 3600)}小时${Math.floor((seconds % 3600) / 60)}分钟`;
};

const DownloadList = () => {
  const dispatch = useDispatch();
  const downloads = useSelector(state => state.downloads.items);
  const loading = useSelector(state => state.downloads.loading);

  useEffect(() => {
    dispatch(fetchDownloads());
  }, [dispatch]);

  const handleAddDownload = (downloadInfo) => {
    dispatch(startDownload(downloadInfo));
  };

  const handlePause = (id) => {
    dispatch(pauseDownload(id));
  };

  const handleResume = (id) => {
    dispatch(resumeDownload(id));
  };

  const handleCancel = (id) => {
    dispatch(cancelDownload(id));
  };

  if (loading) {
    return <LinearProgress />;
  }

  return (
    <Box p={2}>
      <Box display="flex" justifyContent="space-between" alignItems="center" mb={2}>
        <Typography variant="h6">下载任务</Typography>
        <AddDownloadButton onAddDownload={handleAddDownload} />
      </Box>
      
      <TableContainer component={Paper}>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell>文件名</TableCell>
              <TableCell>大小</TableCell>
              <TableCell>进度</TableCell>
              <TableCell>速度</TableCell>
              <TableCell>剩余时间</TableCell>
              <TableCell>状态</TableCell>
              <TableCell>操作</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {downloads.map((download) => (
              <TableRow key={download.id}>
                <TableCell>{download.fileName}</TableCell>
                <TableCell>{formatSize(download.size)}</TableCell>
                <TableCell>
                  <Box display="flex" alignItems="center">
                    <Box width="100%" mr={1}>
                      <LinearProgress variant="determinate" value={download.progress} />
                    </Box>
                    <Box minWidth={35}>
                      <Typography variant="body2" color="textSecondary">
                        {`${Math.round(download.progress)}%`}
                      </Typography>
                    </Box>
                  </Box>
                </TableCell>
                <TableCell>{formatSpeed(download.speed)}</TableCell>
                <TableCell>{formatRemainingTime(download.remainingTime)}</TableCell>
                <TableCell>{download.status}</TableCell>
                <TableCell>
                  {download.status === 'downloading' ? (
                    <IconButton onClick={() => handlePause(download.id)} size="small">
                      <PauseIcon />
                    </IconButton>
                  ) : download.status === 'paused' ? (
                    <IconButton onClick={() => handleResume(download.id)} size="small">
                      <ResumeIcon />
                    </IconButton>
                  ) : null}
                  <IconButton onClick={() => handleCancel(download.id)} size="small">
                    <DeleteIcon />
                  </IconButton>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
    </Box>
  );
};

export default DownloadList; 