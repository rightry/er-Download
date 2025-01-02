import React, { useState } from 'react';
import {
  IconButton,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  TextField,
  Tooltip,
  Box,
  Typography,
} from '@mui/material';
import { Add as AddIcon, Folder as FolderIcon } from '@mui/icons-material';

const AddDownloadButton = ({ onAddDownload }) => {
  const [open, setOpen] = useState(false);
  const [url, setUrl] = useState('');
  const [savePath, setSavePath] = useState('./downloads');
  const [error, setError] = useState('');

  const handleClickOpen = () => {
    // 尝试从剪贴板获取URL
    navigator.clipboard.readText()
      .then(text => {
        if (text.startsWith('http://') || text.startsWith('https://')) {
          setUrl(text);
          setError('');
        }
      })
      .catch(() => {
        // 剪贴板访问被拒绝,忽略错误
      });
    setOpen(true);
  };

  const handleClose = () => {
    setOpen(false);
    setUrl('');
    setSavePath('./downloads');
    setError('');
  };

  const handleSubmit = () => {
    if (!url) {
      setError('请输入下载链接');
      return;
    }
    if (!url.startsWith('http://') && !url.startsWith('https://')) {
      setError('请输入有效的下载链接');
      return;
    }

    onAddDownload({
      url,
      fileName: url.split('/').pop() || 'unnamed',
      savePath,
      priority: 'normal'
    });

    handleClose();
  };

  const handlePaste = () => {
    navigator.clipboard.readText()
      .then(text => {
        setUrl(text);
        setError('');
      })
      .catch(() => {
        setError('无法访问剪贴板');
      });
  };

  const handleSelectDirectory = async () => {
    try {
      const dirHandle = await window.showDirectoryPicker({
        mode: 'readwrite'
      });
      setSavePath(dirHandle.name);
    } catch (err) {
      // 用户取消选择或不支持该API
      console.error('选择目录失败:', err);
    }
  };

  return (
    <>
      <Tooltip title="添加下载">
        <IconButton color="primary" onClick={handleClickOpen}>
          <AddIcon />
        </IconButton>
      </Tooltip>

      <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
        <DialogTitle>添加下载任务</DialogTitle>
        <DialogContent>
          <TextField
            autoFocus
            margin="dense"
            label="下载链接"
            type="url"
            fullWidth
            variant="outlined"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            error={!!error}
            helperText={error}
            onPaste={handlePaste}
          />
          
          <Box mt={2} display="flex" alignItems="center" gap={1}>
            <TextField
              label="保存位置"
              variant="outlined"
              fullWidth
              value={savePath}
              InputProps={{
                readOnly: true,
              }}
            />
            <IconButton onClick={handleSelectDirectory} color="primary">
              <FolderIcon />
            </IconButton>
          </Box>
          
          <Typography variant="caption" color="textSecondary" sx={{ mt: 1, display: 'block' }}>
            点击文件夹图标选择下载位置
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleClose}>取消</Button>
          <Button onClick={handleSubmit} variant="contained" color="primary">
            开始下载
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
};

export default AddDownloadButton; 