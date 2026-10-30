(defcolumns (X :i16) (Y :i16) (Z :i16) (C :i16 :constant 1))
(deflookup test (Y Z) (X C))
