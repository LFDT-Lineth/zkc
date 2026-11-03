;;error:6:13-14:column is untyped
;;error:6:15-17:empty column declaration
;;error:7:16-17:unknown type
;;error:8:16-23:unknown type
;;error:9:13-27:column is untyped
(defcolumns X ())
(defcolumns (Y :))
(defcolumns (Y :@prove))
(defcolumns (Z :padding 0))
