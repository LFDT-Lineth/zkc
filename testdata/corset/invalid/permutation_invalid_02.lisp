;;error:3:19-28:differing number of source and target columns (2 v 1)
(defcolumns (X :i16) (Y :i16))
(defpermutation p ((_ X X)) ((_ Y)))
