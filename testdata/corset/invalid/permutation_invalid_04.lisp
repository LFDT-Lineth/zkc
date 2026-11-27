;;error:3:42-43:non-binary selector encountered
(defcolumns (P :i2) (X :i17) (Y :i16))
(defpermutation (p :unchecked) ((_ X)) ((P Y)))
