#ifndef _INCLUDE_METAMOD_SOURCE_STUB_PLUGIN_H_
#define _INCLUDE_METAMOD_SOURCE_STUB_PLUGIN_H_

#include <ISmmPlugin.h>
#include <eiface.h>

class SourceMapThumbnail : public ISmmPlugin
{
	public:
		bool Load(PluginId id, ISmmAPI *ismm, char *error, size_t maxlength, bool late);
		bool Unload(char *error, size_t maxlen);
	public:
		const char *GetAuthor();
		const char *GetName();
		const char *GetDescription();
		const char *GetURL();
		const char *GetVersion();
		const char *GetDate();
		const char *GetLicense();
		const char *GetLogTag();
};

extern SourceMapThumbnail g_SourceThumbnail;

PLUGIN_GLOBALVARS();

#endif //_INCLUDE_METAMOD_SOURCE_STUB_PLUGIN_H_
