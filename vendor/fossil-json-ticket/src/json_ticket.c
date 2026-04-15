#ifdef FOSSIL_ENABLE_JSON
/*
** JSON API for tickets: list, get, history, save (create/update with J-card field changes)
*/
#include "VERSION.h"
#include "config.h"
#include "json_ticket.h"

#if INTERFACE
#include "json_detail.h"
#endif
cson_value * json_ticket_history(void);

/*
** /json/ticket - dispatch
*/
cson_value * json_page_ticket(void){
  static const JsonPageDef sub[] = {
    {"get", json_ticket_get, 0},
    {"history", json_ticket_history, 0},
    {"list", json_ticket_list, 0},
    {"save", json_ticket_save, 0},
    {NULL,NULL,0}
  };
  return json_page_dispatch_helper(sub);
}

/*
** /json/ticket/list
*/
cson_value * json_ticket_list(void){
  Stmt q;
  cson_value * listV;
  cson_array * list;
  if( !g.perm.RdTkt ){
    json_set_err(FSL_JSON_E_DENIED, "Requires 't' permissions.");
    return NULL;
  }
  listV = cson_value_new_array();
  list = cson_value_get_array(listV);
  db_prepare(&q,
    "SELECT tkt_uuid, title, type, status, datetime(tkt_mtime,toLocal())"
    " FROM ticket ORDER BY tkt_mtime DESC"
  );
  while( db_step(&q)==SQLITE_ROW ){
    cson_value * rowV = cson_value_new_object();
    cson_object * row = cson_value_get_object(rowV);
    cson_object_set(row, "uuid", json_new_string(db_column_text(&q,0)));
    cson_object_set(row, "title", json_new_string(db_column_text(&q,1)));
    cson_object_set(row, "type", json_new_string(db_column_text(&q,2)));
    cson_object_set(row, "status", json_new_string(db_column_text(&q,3)));
    cson_object_set(row, "mtime", json_new_string(db_column_text(&q,4)));
    cson_array_append(list, rowV);
  }
  db_finalize(&q);
  return listV;
}

/*
** /json/ticket/get?uuid=TICKETUUID
*/
cson_value * json_ticket_get(void){
  const char *zUuid;
  Stmt q;
  cson_value * payV;
  cson_object * pay;
  int i, nCol;
  if( !g.perm.RdTkt ){
    json_set_err(FSL_JSON_E_DENIED, "Requires 't' permissions.");
    return NULL;
  }
  zUuid = json_find_option_cstr("uuid", NULL, NULL);
  if(!zUuid || !*zUuid){
    json_set_err(FSL_JSON_E_MISSING_ARGS, "'uuid' parameter is required.");
    return NULL;
  }
  db_prepare(&q, "SELECT * FROM ticket WHERE tkt_uuid GLOB '%q*'", zUuid);
  if( db_step(&q)!=SQLITE_ROW ){
    db_finalize(&q);
    json_set_err(FSL_JSON_E_RESOURCE_NOT_FOUND, "Ticket not found.");
    return NULL;
  }
  payV = cson_value_new_object();
  pay = cson_value_get_object(payV);
  nCol = db_column_count(&q);
  for(i=0; i<nCol; i++){
    const char *zName = db_column_name(&q, i);
    const char *zVal = db_column_text(&q, i);
    if(zVal){
      cson_object_set(pay, zName, json_new_string(zVal));
    }else{
      cson_object_set(pay, zName, cson_value_null());
    }
  }
  db_finalize(&q);
  return payV;
}

/*
** /json/ticket/history?uuid=TICKETUUID
** Chronological TICKETCHNG rows: mtime, login, username, mimetype, icomment
** (icomment may be empty for field-only updates).
*/
cson_value * json_ticket_history(void){
  const char *zUuid;
  Stmt q;
  cson_value * listV;
  cson_array * list;
  if( !g.perm.RdTkt ){
    json_set_err(FSL_JSON_E_DENIED, "Requires 't' permissions.");
    return NULL;
  }
  zUuid = json_find_option_cstr("uuid", NULL, NULL);
  if(!zUuid || !*zUuid){
    json_set_err(FSL_JSON_E_MISSING_ARGS, "'uuid' parameter is required.");
    return NULL;
  }
  db_prepare(&q, "SELECT 1 FROM ticket WHERE tkt_uuid GLOB '%q*'", zUuid);
  if( db_step(&q)!=SQLITE_ROW ){
    db_finalize(&q);
    json_set_err(FSL_JSON_E_RESOURCE_NOT_FOUND, "Ticket not found.");
    return NULL;
  }
  db_finalize(&q);
  listV = cson_value_new_array();
  list = cson_value_get_array(listV);
  db_prepare(&q,
    "SELECT datetime(c.tkt_mtime,toLocal()) AS mtime, c.login, c.username, c.mimetype,"
    " coalesce(c.icomment,'') AS icomment"
    " FROM ticketchng AS c"
    " JOIN ticket AS t ON t.tkt_id=c.tkt_id"
    " WHERE t.tkt_uuid GLOB '%q*'"
    " ORDER BY c.tkt_mtime ASC",
    zUuid
  );
  while( db_step(&q)==SQLITE_ROW ){
    cson_value * rowV = cson_value_new_object();
    cson_object * row = cson_value_get_object(rowV);
    cson_object_set(row, "mtime", json_new_string(db_column_text(&q,0)));
    cson_object_set(row, "login", json_new_string(db_column_text(&q,1)));
    cson_object_set(row, "username", json_new_string(db_column_text(&q,2)));
    cson_object_set(row, "mimetype", json_new_string(db_column_text(&q,3)));
    cson_object_set(row, "icomment", json_new_string(db_column_text(&q,4)));
    cson_array_append(list, rowV);
  }
  db_finalize(&q);
  return listV;
}

/*
** /json/ticket/save
** Payload fields become J-card entries in a ticket change artifact.
** If "uuid" is in payload -> update existing ticket.
** Otherwise -> create new ticket.
** Use "+fieldname" to append (J+ card) instead of replace.
*/
cson_value * json_ticket_save(void){
  cson_value * uuidV;
  const char * zTktUuid;
  const char * zUser;
  Blob tktchng = empty_blob;
  Blob cksum = empty_blob;
  char *aUsed = 0;
  int i;
  int nJ;
  int isNew = 0;
  cson_object * reqPayload;
  cson_value * resultV;
  cson_object * result;

  if( !g.perm.WrTkt && !g.perm.NewTkt ){
    json_set_err(FSL_JSON_E_DENIED, "Requires 'n' or 'w' permissions.");
    return NULL;
  }
  reqPayload = g.json.reqPayload.o;
  if(!reqPayload){
    json_set_err(FSL_JSON_E_MISSING_ARGS, "Payload object is required.");
    return NULL;
  }

  uuidV = cson_object_get(reqPayload, "uuid");
  if(uuidV && cson_value_is_string(uuidV)){
    zTktUuid = cson_string_cstr(cson_value_get_string(uuidV));
    zTktUuid = db_text(0,
      "SELECT tkt_uuid FROM ticket WHERE tkt_uuid GLOB '%q*'", zTktUuid);
    if(!zTktUuid){
      json_set_err(FSL_JSON_E_RESOURCE_NOT_FOUND, "Ticket not found.");
      return NULL;
    }
  }else{
    if( !g.perm.NewTkt ){
      json_set_err(FSL_JSON_E_DENIED, "Requires 'n' permissions.");
      return NULL;
    }
    zTktUuid = db_text(0, "SELECT lower(hex(randomblob(20)))");
    isNew = 1;
  }

  zUser = g.zLogin ? g.zLogin : "anonymous";
  if(isNew) ticket_init();
  getAllTicketFields();
  aUsed = fossil_malloc_zero(nField);

  blob_appendf(&tktchng, "D %s\n", date_in_standard_format("now"));

  /*
  ** J-cards must appear in ascending field-name order (see manifest.c J-card
  ** handling). Iterate aField[] in definition order, like submit_ticketCmd
  ** in tkt.c: append (+field) entries first, then assignments.
  */
  nJ = 0;
  for(i=0; i<nField; i++){
    char *zPlusKey = mprintf("+%s", aField[i].zName);
    cson_value *vVal = cson_object_get(reqPayload, zPlusKey);
    const char *zVal;
    fossil_free(zPlusKey);
    if(!vVal || !cson_value_is_string(vVal)) continue;
    zVal = cson_string_cstr(cson_value_get_string(vVal));
    if(!zVal) zVal = "";
    blob_appendf(&tktchng, "J +%s %#F\n", aField[i].zName,
                 (int)strlen(zVal), zVal);
    aUsed[i] = '+';
    nJ++;
  }
  for(i=0; i<nField; i++){
    cson_value *vVal;
    const char *zVal;
    if(aUsed[i]=='+') continue;
    /* Reserved for ticket/update identity; not a ticket column change. */
    if(fossil_strcmp(aField[i].zName, "uuid")==0) continue;
    vVal = cson_object_get(reqPayload, aField[i].zName);
    if(!vVal || !cson_value_is_string(vVal)) continue;
    zVal = cson_string_cstr(cson_value_get_string(vVal));
    if(!zVal) zVal = "";
    blob_appendf(&tktchng, "J %s %#F\n", aField[i].zName,
                 (int)strlen(zVal), zVal);
    aUsed[i] = '=';
    nJ++;
  }

  if( nJ==0 ){
    json_set_err(FSL_JSON_E_MISSING_ARGS, "At least one ticket field change is required.");
    fossil_free(aUsed);
    return NULL;
  }

  blob_appendf(&tktchng, "K %s\n", zTktUuid);
  blob_appendf(&tktchng, "U %F\n", zUser);
  md5sum_blob(&tktchng, &cksum);
  blob_appendf(&tktchng, "Z %b\n", &cksum);

  if( ticket_put(&tktchng, zTktUuid, aUsed,
                  ticket_need_moderation(0))==0 ){
    json_set_err(FSL_JSON_E_UNKNOWN, "Ticket save failed.");
    fossil_free(aUsed);
    return NULL;
  }
  ticket_change(zTktUuid);

  fossil_free(aUsed);
  resultV = cson_value_new_object();
  result = cson_value_get_object(resultV);
  cson_object_set(result, "uuid", json_new_string(zTktUuid));
  cson_object_set(result, "isNew",
    cson_value_new_integer(isNew ? 1 : 0));
  return resultV;
}

#endif /* FOSSIL_ENABLE_JSON */
